package services

import (
	"regexp"
	"strings"
)

// manifestProgressTracker 统一 manifest AI 两个流程的分步进度推送 + 各步耗时记账。
// 取代两个 service 里重复的 reportProgress/completedSteps/lastStepTimer 闭包。
type manifestProgressTracker struct {
	totalSteps     int
	stepName       func(step int) string // 各步骤名(用于回填上一步)
	callback       ProgressCallback
	completedSteps []CompletedStep
	lastStepTimer  *Timer
}

func newManifestProgressTracker(totalSteps int, stepName func(int) string, cb ProgressCallback) *manifestProgressTracker {
	return &manifestProgressTracker{totalSteps: totalSteps, stepName: stepName, callback: cb}
}

// report 进入第 step 步：先回填上一步耗时，再推送当前步的 progress 事件。
func (t *manifestProgressTracker) report(step int, stepName, message string) {
	if t.lastStepTimer != nil && step > 1 && len(t.completedSteps) < step-1 {
		t.completedSteps = append(t.completedSteps, CompletedStep{
			Name:      t.stepName(step - 1),
			ElapsedMs: int64(t.lastStepTimer.ElapsedMs()),
		})
	}
	t.lastStepTimer = NewTimer()

	if t.callback != nil {
		t.callback(ProgressEvent{
			Type:           "progress",
			Step:           step,
			TotalSteps:     t.totalSteps,
			StepName:       stepName,
			Message:        message,
			CompletedSteps: t.completedSteps,
		})
	}
}

// addStep 直接追加一条已完成步骤(用于最后一步:带自身耗时与使用的 Skills)。
func (t *manifestProgressTracker) addStep(name string, elapsedMs int64, usedSkills []string) {
	t.completedSteps = append(t.completedSteps, CompletedStep{
		Name:       name,
		ElapsedMs:  elapsedMs,
		UsedSkills: usedSkills,
	})
}

// steps 返回累计的已完成步骤(供 service 放进最终结果,让 complete 事件带全量 pipeline)。
func (t *manifestProgressTracker) steps() []CompletedStep {
	return t.completedSteps
}

// hclFenceRe 匹配代码块起始围栏，捕获语言标识(可空)到行尾。
// 例如 ```hcl / ```terraform / ```tfvars / ``` 都能匹配，避免 "```tf" 误匹配 "```tfvars"。
var hclFenceRe = regexp.MustCompile("(?m)^[ \\t]*```[ \\t]*([A-Za-z0-9_-]*)[ \\t]*\\r?\\n")

// extractHCL 从 AI 响应中提取 HCL 文本
// 支持带语言标识(hcl/terraform/tf/tfvars 等)或裸 ``` 代码块；无代码块时返回原文。
func extractHCL(text string) string {
	text = cleanInvalidChars(text)

	// 找第一个代码块起始围栏(整行 ```lang 形式),取其后到结束围栏之间的内容。
	if loc := hclFenceRe.FindStringIndex(text); loc != nil {
		rest := text[loc[1]:] // 围栏行(含换行)之后
		if endIdx := strings.Index(rest, "```"); endIdx >= 0 {
			return strings.TrimSpace(rest[:endIdx])
		}
		return strings.TrimSpace(rest)
	}

	return strings.TrimSpace(text)
}
