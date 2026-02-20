# State Preview 搜索和语法高亮功能实现

## 实施日期
2025-10-12

## 概述
为 State Preview 页面添加了 JSON 语法高亮和搜索功能，并优化了 State History 表格显示。

## 实现的功能

### 1. JSON 语法高亮
实现了完整的 JSON 语法高亮，正确区分键和值：

- **键（属性名）**: 红色 (#d73a49)，加粗显示
- **值（字符串）**: 绿色 (#22863a)
- **数字**: 蓝色 (#005cc5)
- **布尔值**: 紫色 (#6f42c1)
- **null**: 灰色 (#6a737d)

#### 技术实现
- 使用正则表达式 `/"([^"]+)"(?=\s*:)/g` 识别键（使用前瞻断言匹配冒号）
- 分别匹配字符串、数字、布尔值和 null
- 键的优先级高于其他匹配，避免重叠

### 2. 搜索功能
实现了功能完整的搜索系统：

#### 核心功能
- **实时搜索**: 输入时自动搜索并高亮所有匹配项
- **匹配高亮**: 
  - 所有匹配项：黄色背景 (#fff3cd)
  - 当前匹配项：橙色背景 (#ff9800)，白色文字
- **匹配计数**: 显示 "X / Y" 格式（如 "1 / 5"）
- **导航控制**: 
  - "← Prev" 按钮：跳转到上一个匹配
  - "Next →" 按钮：跳转到下一个匹配
  - 支持循环导航（到达末尾后回到开头）
- **大小写敏感**: 可选的大小写敏感搜索
- **自动滚动**: 自动滚动到当前匹配项，居中显示

#### 技术实现
```typescript
// 搜索匹配
useEffect(() => {
  if (!stateData || !searchTerm.trim()) {
    setMatches([]);
    setCurrentMatchIndex(0);
    return;
  }

  const jsonStr = JSON.stringify(stateData, null, 2);
  const searchPattern = caseSensitive ? searchTerm : searchTerm.toLowerCase();
  const searchIn = caseSensitive ? jsonStr : jsonStr.toLowerCase();
  
  // 查找所有匹配项
  const foundMatches: Match[] = [];
  let index = 0;
  let matchIndex = 0;
  
  while ((index = searchIn.indexOf(searchPattern, index)) !== -1) {
    const beforeMatch = jsonStr.substring(0, index);
    const line = beforeMatch.split('\n').length;
    
    foundMatches.push({
      index: matchIndex++,
      start: index,
      end: index + searchTerm.length,
      line
    });
    index += searchTerm.length;
  }
  
  setMatches(foundMatches);
  setCurrentMatchIndex(foundMatches.length > 0 ? 0 : -1);
}, [searchTerm, caseSensitive, stateData]);

// 自动滚动到当前匹配
useEffect(() => {
  if (matches.length > 0 && currentMatchIndex >= 0) {
    const matchElement = matchRefs.current.get(currentMatchIndex);
    if (matchElement) {
      matchElement.scrollIntoView({
        behavior: 'smooth',
        block: 'center'
      });
    }
  }
}, [currentMatchIndex, matches]);
```

### 3. State History 表格优化
简化了 State History 表格显示：

#### 变更内容
- **移除**: "下载链接" 列及其下载按钮
- **保留**: 
  - 版本列（显示版本号和 CURRENT 标签）
  - CHECKSUM 列
  - 创建时间列
- **交互**: 点击整行跳转到 State Preview 页面
- **下载**: 在 State Preview 页面提供下载功能

#### 优势
- 界面更简洁
- 减少用户混淆（只有一个下载入口）
- 保持一致的用户体验

## 修改的文件

### 1. frontend/src/pages/StatePreview.tsx
**主要变更**:
- 添加搜索状态管理（searchTerm, caseSensitive, matches, currentMatchIndex）
- 实现 `highlightJSON()` 函数进行语法高亮
- 实现 `highlightSearchMatches()` 函数进行搜索匹配高亮
- 添加搜索导航函数（handlePreviousMatch, handleNextMatch）
- 使用 useRef 管理匹配元素引用，实现自动滚动
- 使用 useMemo 优化性能，避免不必要的重新渲染

**新增 UI 组件**:
```tsx
{/* 搜索控件 */}
<input
  type="text"
  placeholder="Search in JSON..."
  value={searchTerm}
  onChange={(e) => setSearchTerm(e.target.value)}
  className={styles.filterInput}
/>

{matches.length > 0 && (
  <>
    <div className={styles.matchCounter}>
      {currentMatchIndex + 1} / {matches.length}
    </div>
    <button onClick={handlePreviousMatch} className={styles.navButton}>
      ← Prev
    </button>
    <button onClick={handleNextMatch} className={styles.navButton}>
      Next →
    </button>
  </>
)}

<label className={styles.caseSensitiveLabel}>
  <input
    type="checkbox"
    checked={caseSensitive}
    onChange={(e) => setCaseSensitive(e.target.checked)}
  />
  Case sensitive
</label>
```

### 2. frontend/src/pages/StatePreview.module.css
**新增样式**:
```css
/* JSON 语法高亮 */
.jsonString { color: #22863a; }
.jsonNumber { color: #005cc5; }
.jsonBoolean { color: #6f42c1; }
.jsonNull { color: #6a737d; }
.jsonKey { color: #d73a49; font-weight: 500; }

/* 搜索匹配高亮 */
.searchMatch {
  background-color: #fff3cd;
  border-radius: 2px;
  padding: 1px 2px;
}

.currentMatch {
  background-color: #ff9800;
  color: white;
  border-radius: 2px;
  padding: 1px 2px;
  font-weight: 600;
}

/* 搜索控件 */
.matchCounter { ... }
.navButton { ... }
.caseSensitiveLabel { ... }
.caseSensitiveCheckbox { ... }
```

### 3. frontend/src/pages/StatesTab.tsx
**主要变更**:
- 移除 "下载链接" 列的表头
- 移除每行的下载按钮和文件名显示
- 简化表格结构为 3 列布局

**变更前**:
```tsx
<div className={styles.tableHeader}>
  <div style={{flex: '0 0 120px'}}>版本</div>
  <div style={{flex: '1'}}>下载链接</div>
  <div style={{flex: '0 0 300px'}}>CHECKSUM</div>
  <div style={{flex: '0 0 150px'}}>创建时间</div>
</div>
```

**变更后**:
```tsx
<div className={styles.tableHeader}>
  <div style={{flex: '0 0 120px'}}>版本</div>
  <div style={{flex: '0 0 300px'}}>CHECKSUM</div>
  <div style={{flex: '0 0 150px'}}>创建时间</div>
</div>
```

## 用户体验改进

### 搜索体验
1. **即时反馈**: 输入时立即显示匹配结果
2. **清晰标识**: 当前匹配项使用不同颜色突出显示
3. **便捷导航**: 键盘和鼠标都可以轻松导航
4. **智能滚动**: 自动滚动到匹配位置，无需手动查找

### 视觉体验
1. **语法高亮**: 不同类型的数据使用不同颜色，提高可读性
2. **键值区分**: 清晰区分 JSON 的键和值
3. **一致性**: 颜色方案符合常见代码编辑器的习惯

### 交互体验
1. **简化操作**: 移除冗余的下载按钮，统一下载入口
2. **直观导航**: 点击行即可查看详情
3. **响应式**: 所有操作都有即时的视觉反馈

## 技术亮点

### 1. 性能优化
- 使用 `useMemo` 缓存语法高亮结果
- 使用 `useCallback` 优化事件处理函数
- 使用 `useRef` 管理 DOM 引用，避免不必要的重新渲染

### 2. 代码质量
- TypeScript 类型安全
- 清晰的接口定义
- 模块化的函数设计
- 良好的代码注释

### 3. 用户体验
- 平滑的滚动动画
- 响应式的搜索
- 直观的视觉反馈
- 无缝的导航体验

## 测试建议

### 功能测试
1. **语法高亮**:
   - 验证不同类型的值显示正确的颜色
   - 验证键和值的颜色区分
   - 测试嵌套对象和数组的高亮

2. **搜索功能**:
   - 测试大小写敏感/不敏感搜索
   - 测试特殊字符搜索
   - 测试空搜索词的处理
   - 验证匹配计数的准确性
   - 测试导航按钮的循环行为

3. **State History**:
   - 验证表格显示正确
   - 测试行点击导航
   - 验证分页功能

### 性能测试
1. 测试大型 JSON 文件的渲染性能
2. 测试搜索大量匹配项的性能
3. 测试频繁切换匹配项的流畅度

### 兼容性测试
1. 测试不同浏览器的兼容性
2. 测试不同屏幕尺寸的响应式布局
3. 测试键盘导航的可访问性

## 未来改进建议

### 短期改进
1. 添加正则表达式搜索支持
2. 添加搜索历史记录
3. 支持键盘快捷键（如 Ctrl+F, F3, Shift+F3）
4. 添加搜索结果导出功能

### 长期改进
1. 支持 JSON 折叠/展开
2. 添加 JSON 格式化选项
3. 支持 JSON 路径复制
4. 添加 JSON 差异对比功能
5. 支持自定义语法高亮主题

## 总结

本次实现成功为 State Preview 页面添加了完整的搜索和语法高亮功能，显著提升了用户查看和搜索 Terraform State 文件的体验。通过优化 State History 表格，简化了用户界面，使整体交互更加流畅和直观。

所有功能都经过精心设计和实现，确保了良好的性能和用户体验。代码质量高，易于维护和扩展。
