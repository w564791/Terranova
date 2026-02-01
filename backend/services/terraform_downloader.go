package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// TerraformDownloader IaC引擎二进制下载器（支持Terraform和OpenTofu）
type TerraformDownloader struct {
	db              *gorm.DB
	versionService  *TerraformVersionService
	downloadBaseDir string              // 下载目录基础路径
	remoteAccessor  *RemoteDataAccessor // Agent模式下使用API访问
}

// NewTerraformDownloader 创建Terraform下载器（Local模式）
func NewTerraformDownloader(db *gorm.DB) *TerraformDownloader {
	return &TerraformDownloader{
		db:              db,
		versionService:  NewTerraformVersionService(db),
		downloadBaseDir: "/tmp/iac-platform/terraform-binaries", // 默认下载目录（与workspace工作目录在同一基础路径下）
	}
}

// NewTerraformDownloaderForAgent 创建Terraform下载器（Agent模式）
func NewTerraformDownloaderForAgent(accessor *RemoteDataAccessor) *TerraformDownloader {
	return &TerraformDownloader{
		db:              nil, // Agent模式不使用数据库
		versionService:  nil, // Agent模式使用API获取版本信息
		downloadBaseDir: "/tmp/iac-platform/terraform-binaries",
		remoteAccessor:  accessor, // 新增：用于Agent模式的API访问
	}
}

// SetDownloadBaseDir 设置下载目录
func (d *TerraformDownloader) SetDownloadBaseDir(dir string) {
	d.downloadBaseDir = dir
}

// EnsureTerraformBinary 确保Terraform二进制文件存在并可用
// 如果不存在则下载，如果存在则验证
func (d *TerraformDownloader) EnsureTerraformBinary(version string) (string, error) {
	log.Printf("Ensuring Terraform binary for version: %s", version)

	// 1. 处理特殊版本标识并获取版本配置
	var tfVersion models.TerraformVersion
	var err error

	if version == "" || version == "latest" {
		// 获取默认版本
		if d.versionService != nil {
			// Local模式：使用versionService
			defaultVersion, err := d.versionService.GetDefault()
			if err != nil {
				return "", fmt.Errorf("failed to get default terraform version: %w", err)
			}
			tfVersion = *defaultVersion
			version = defaultVersion.Version
		} else if d.remoteAccessor != nil {
			// Agent模式：通过API获取默认版本
			defaultVersion, err := d.getTerraformVersionViaAPI("latest")
			if err != nil {
				return "", fmt.Errorf("failed to get default terraform version via API: %w", err)
			}
			tfVersion = *defaultVersion
			version = defaultVersion.Version
		} else {
			return "", fmt.Errorf("no version service or remote accessor available")
		}
		log.Printf("Using default version: %s", version)
	} else {
		// 2. 检查版本配置是否存在
		if d.db != nil {
			// Local模式：从数据库查询
			err = d.db.Where("version = ? AND enabled = ?", version, true).First(&tfVersion).Error
			if err == gorm.ErrRecordNotFound {
				return "", fmt.Errorf("terraform version %s not found or not enabled in configuration", version)
			}
			if err != nil {
				return "", fmt.Errorf("failed to query terraform version: %w", err)
			}
		} else if d.remoteAccessor != nil {
			// Agent模式：通过API获取
			versionConfig, err := d.getTerraformVersionViaAPI(version)
			if err != nil {
				return "", fmt.Errorf("failed to get terraform version %s via API: %w", version, err)
			}
			tfVersion = *versionConfig
		} else {
			return "", fmt.Errorf("no database or remote accessor available")
		}
	}

	// 3. 检查二进制文件是否已存在
	binaryPath := d.GetBinaryPath(version)
	if d.isBinaryValid(binaryPath, "") {
		log.Printf("Terraform binary already exists and is valid: %s", binaryPath)
		return binaryPath, nil
	}

	// 4. 下载并安装
	log.Printf("Downloading Terraform %s from %s", version, tfVersion.DownloadURL)
	if err := d.downloadAndInstall(&tfVersion); err != nil {
		return "", fmt.Errorf("failed to download terraform %s: %w", version, err)
	}

	// 5. 再次验证（只验证可执行性，不验证 checksum）
	if !d.isBinaryValid(binaryPath, "") {
		return "", fmt.Errorf("downloaded binary failed validation")
	}

	log.Printf("Terraform %s is ready at: %s", version, binaryPath)
	return binaryPath, nil
}

// GetBinaryPath 获取二进制文件路径（公开方法）
// 注意：此方法返回的是默认的terraform路径，如需获取正确的引擎路径，请使用 GetBinaryPathForVersion
func (d *TerraformDownloader) GetBinaryPath(version string) string {
	return filepath.Join(d.downloadBaseDir, version, "terraform")
}

// GetBinaryPathForVersion 根据版本配置获取正确的二进制文件路径
func (d *TerraformDownloader) GetBinaryPathForVersion(tfVersion *models.TerraformVersion) string {
	binaryName := tfVersion.GetEngineType().GetBinaryName()
	return filepath.Join(d.downloadBaseDir, tfVersion.Version, binaryName)
}

// isBinaryValid 检查二进制文件是否有效
func (d *TerraformDownloader) isBinaryValid(binaryPath string, expectedChecksum string) bool {
	// 检查文件是否存在
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return false
	}

	// 检查文件是否可执行
	if err := d.checkExecutable(binaryPath); err != nil {
		log.Printf("Binary exists but is not executable: %v", err)
		return false
	}

	// 注意：expectedChecksum 是 ZIP 文件的 checksum，不是二进制文件的 checksum
	// 所以这里不验证二进制文件的 checksum，只验证可执行性
	// ZIP 文件的 checksum 在下载时已经验证过了

	return true
}

// checkExecutable 检查文件是否可执行
func (d *TerraformDownloader) checkExecutable(path string) error {
	// 尝试执行 terraform version
	cmd := exec.Command(path, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute terraform: %w, output: %s", err, string(output))
	}
	// 打印版本信息用于验证
	log.Printf("Terraform binary verified: %s", strings.TrimSpace(string(output)))
	return nil
}

// calculateFileChecksum 计算文件的SHA256校验和
func (d *TerraformDownloader) calculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// downloadAndInstall 下载并安装IaC引擎二进制文件（支持Terraform和OpenTofu）
func (d *TerraformDownloader) downloadAndInstall(version *models.TerraformVersion) error {
	// 获取引擎类型和二进制名称（运行时推断）
	engineType := version.GetEngineType()
	binaryName := engineType.GetBinaryName()
	displayName := engineType.GetDisplayName()

	log.Printf("Installing %s %s (engine: %s)", displayName, version.Version, engineType)

	// 1. 创建临时下载目录
	tempDir, err := os.MkdirTemp("", "iac-download-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// 2. 下载文件
	downloadPath := filepath.Join(tempDir, "iac.zip")
	log.Printf("Downloading to: %s", downloadPath)

	if err := d.downloadFile(version.DownloadURL, downloadPath); err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	// 3. 验证下载文件的checksum
	log.Printf("Verifying download checksum...")
	actualChecksum, err := d.calculateFileChecksum(downloadPath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	if actualChecksum != version.Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", version.Checksum, actualChecksum)
	}
	log.Printf("Checksum verified successfully")

	// 4. 解压文件
	log.Printf("Extracting archive...")
	extractDir := filepath.Join(tempDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return fmt.Errorf("failed to create extract directory: %w", err)
	}

	if err := d.extractZip(downloadPath, extractDir); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	// 5. 找到二进制文件（根据引擎类型）
	sourceBinary := filepath.Join(extractDir, binaryName)
	if runtime.GOOS == "windows" {
		sourceBinary += ".exe"
	}

	// 如果找不到对应的二进制文件，尝试查找其他可能的名称
	if _, err := os.Stat(sourceBinary); os.IsNotExist(err) {
		// 尝试查找 terraform（某些 OpenTofu 版本可能仍使用 terraform 名称）
		altBinary := filepath.Join(extractDir, "terraform")
		if runtime.GOOS == "windows" {
			altBinary += ".exe"
		}
		if _, err := os.Stat(altBinary); err == nil {
			sourceBinary = altBinary
			log.Printf("Using alternative binary name: terraform")
		} else {
			return fmt.Errorf("%s binary not found in archive (tried: %s, terraform)", binaryName, binaryName)
		}
	}

	// 6. 创建目标目录（使用绝对路径并确保父目录存在）
	targetDir := filepath.Join(d.downloadBaseDir, version.Version)

	// 确保父目录存在
	if err := os.MkdirAll(d.downloadBaseDir, 0755); err != nil {
		return fmt.Errorf("failed to create base directory %s: %w", d.downloadBaseDir, err)
	}

	// 创建版本目录
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory %s: %w", targetDir, err)
	}

	// 验证目录创建成功
	if stat, err := os.Stat(targetDir); err != nil {
		return fmt.Errorf("target directory %s does not exist after creation: %w", targetDir, err)
	} else if !stat.IsDir() {
		return fmt.Errorf("target path %s exists but is not a directory", targetDir)
	}

	log.Printf("Target directory created/verified: %s", targetDir)

	// 7. 移动二进制文件到目标位置
	// 注意：为了保持向后兼容，我们统一使用 "terraform" 作为目标文件名
	// 这样 TerraformExecutor 不需要修改就能正常工作
	targetPath := filepath.Join(targetDir, "terraform")
	if runtime.GOOS == "windows" {
		targetPath += ".exe"
	}

	// 如果目标文件已存在，先删除
	if _, err := os.Stat(targetPath); err == nil {
		log.Printf("Removing existing binary at: %s", targetPath)
		if err := os.Remove(targetPath); err != nil {
			return fmt.Errorf("failed to remove existing binary: %w", err)
		}
	}

	// 复制文件
	log.Printf("Copying binary from %s to %s", sourceBinary, targetPath)
	if err := d.copyFile(sourceBinary, targetPath); err != nil {
		return fmt.Errorf("failed to copy binary from %s to %s: %w", sourceBinary, targetPath, err)
	}

	// 验证文件复制成功
	if stat, err := os.Stat(targetPath); err != nil {
		return fmt.Errorf("target file %s does not exist after copy: %w", targetPath, err)
	} else {
		log.Printf("Binary copied successfully, size: %d bytes", stat.Size())
	}

	// 8. 设置可执行权限（Unix系统）
	if runtime.GOOS != "windows" {
		log.Printf("Setting executable permission on: %s", targetPath)
		if err := os.Chmod(targetPath, 0755); err != nil {
			return fmt.Errorf("failed to set executable permission on %s: %w", targetPath, err)
		}

		// 验证权限设置成功
		if stat, err := os.Stat(targetPath); err != nil {
			return fmt.Errorf("failed to verify permissions on %s: %w", targetPath, err)
		} else {
			log.Printf("Permissions set successfully: %s", stat.Mode().String())
		}
	}

	// 9. 验证安装并打印版本号
	cmd := exec.Command(targetPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Warning: failed to verify %s version: %v", displayName, err)
	} else {
		log.Printf("%s %s installed successfully to: %s", displayName, version.Version, targetPath)
		log.Printf("%s version output: %s", displayName, strings.TrimSpace(string(output)))
	}

	return nil
}

// downloadFile 下载文件
func (d *TerraformDownloader) downloadFile(url, filepath string) error {
	// 创建HTTP客户端
	client := &http.Client{}

	// 发送GET请求
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// 创建文件
	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// 写入文件，显示进度
	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	log.Printf("Downloaded %d bytes", written)
	return nil
}

// extractZip 解压ZIP文件
func (d *TerraformDownloader) extractZip(zipPath, destDir string) error {
	// 使用unzip命令解压（更可靠）
	cmd := exec.Command("unzip", "-o", zipPath, "-d", destDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unzip failed: %w, output: %s", err, string(output))
	}
	return nil
}

// copyFile 复制文件
func (d *TerraformDownloader) copyFile(src, dst string) error {
	// 打开源文件
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", src, err)
	}
	defer sourceFile.Close()

	// 获取源文件信息
	srcInfo, err := sourceFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file %s: %w", src, err)
	}

	// 创建目标文件
	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %w", dst, err)
	}
	defer destFile.Close()

	// 复制文件内容
	written, err := io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	// 验证复制的字节数
	if written != srcInfo.Size() {
		return fmt.Errorf("incomplete copy: expected %d bytes, wrote %d bytes", srcInfo.Size(), written)
	}

	// 同步到磁盘，确保数据写入
	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync file to disk: %w", err)
	}

	log.Printf("Successfully copied %d bytes from %s to %s", written, src, dst)
	return nil
}

// GetInstalledVersions 获取已安装的版本列表
func (d *TerraformDownloader) GetInstalledVersions() ([]string, error) {
	var versions []string

	// 检查下载目录是否存在
	if _, err := os.Stat(d.downloadBaseDir); os.IsNotExist(err) {
		return versions, nil
	}

	// 读取目录
	entries, err := os.ReadDir(d.downloadBaseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read download directory: %w", err)
	}

	// 遍历子目录
	for _, entry := range entries {
		if entry.IsDir() {
			// 检查是否包含terraform二进制文件
			binaryPath := filepath.Join(d.downloadBaseDir, entry.Name(), "terraform")
			if _, err := os.Stat(binaryPath); err == nil {
				versions = append(versions, entry.Name())
			}
		}
	}

	return versions, nil
}

// CleanupVersion 清理指定版本的二进制文件
func (d *TerraformDownloader) CleanupVersion(version string) error {
	versionDir := filepath.Join(d.downloadBaseDir, version)

	// 检查目录是否存在
	if _, err := os.Stat(versionDir); os.IsNotExist(err) {
		return nil // 已经不存在，无需清理
	}

	// 删除目录
	if err := os.RemoveAll(versionDir); err != nil {
		return fmt.Errorf("failed to remove version directory: %w", err)
	}

	log.Printf("Cleaned up Terraform version: %s", version)
	return nil
}

// GetTerraformCommand 获取指定版本的terraform命令路径
// 如果版本不存在，会自动下载
func (d *TerraformDownloader) GetTerraformCommand(version string) (string, error) {
	return d.EnsureTerraformBinary(version)
}

// getTerraformVersionViaAPI 通过API获取Terraform版本配置（Agent模式）
func (d *TerraformDownloader) getTerraformVersionViaAPI(version string) (*models.TerraformVersion, error) {
	if d.remoteAccessor == nil {
		return nil, fmt.Errorf("remote accessor not available")
	}

	// 调用API获取版本配置
	// 这里需要在 AgentAPIClient 中添加相应的方法
	return d.remoteAccessor.apiClient.GetTerraformVersion(version)
}

// IsOpenTofuVersion 检测版本是否为 OpenTofu
// 通过查询版本配置的 download_url 来判断
func (d *TerraformDownloader) IsOpenTofuVersion(version string) bool {
	if d.remoteAccessor == nil {
		return false
	}

	// 通过 API 获取版本配置
	tfVersion, err := d.getTerraformVersionViaAPI(version)
	if err != nil {
		return false
	}

	return tfVersion.GetEngineType() == models.IaCEngineOpenTofu
}

// ValidateDownloadURL 验证下载URL是否有效
func (d *TerraformDownloader) ValidateDownloadURL(url string) error {
	// 发送HEAD请求检查URL是否可访问
	resp, err := http.Head(url)
	if err != nil {
		return fmt.Errorf("failed to access URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("URL returned status: %s", resp.Status)
	}

	// 检查Content-Type是否为zip
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "zip") && !strings.Contains(contentType, "octet-stream") {
		log.Printf("Warning: unexpected content type: %s", contentType)
	}

	return nil
}
