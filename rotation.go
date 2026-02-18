package reco

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (l *Logger) rotateFile() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil || !l.config.EnableRotation {
		return fmt.Errorf("rotation not enabled or not a file logger")
	}

	currentFilePath := l.config.FilePath
if err := l.file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Logger: error closing current log file for rotation: %v\n", err)
	}

	ext := filepath.Ext(currentFilePath)
	base := strings.TrimSuffix(currentFilePath, ext)
	timestamp := time.Now().Format("20060102_150405_000")
	backupPath := fmt.Sprintf("%s.%s%s", base, timestamp, ext)

	if err := os.Rename(currentFilePath, backupPath); err != nil {
		fmt.Fprintf(os.Stderr, "Logger: error renaming log file: %v\n", err)
	}

	newFile, err := os.OpenFile(currentFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		l.writer = os.Stderr
		l.file = nil
		return fmt.Errorf("failed to create new log file after rotation: %w", err)
	}
	l.file = newFile
	l.writer = newFile
	l.currentFileSize.Store(0)

	if l.config.MaxBackups >= 0 {
		go l.cleanupAndCompressBackups(filepath.Dir(currentFilePath), filepath.Base(base), ext, backupPath)
	}
	return nil
}

type backupFileEntry struct {
	Path         string
	ModTime      time.Time
	IsCompressed bool
}

func (l *Logger) cleanupAndCompressBackups(dir, baseName, ext, backupPathJustRotated string) {
	globPattern := filepath.Join(dir, fmt.Sprintf("%s.*%s*", baseName, ext))
	files, err := filepath.Glob(globPattern)
	if err != nil {
		return
	}

	var backups []backupFileEntry
	for _, file := range files {
		if filepath.Base(file) == filepath.Base(l.config.FilePath) {
			continue
		}
		info, statErr := os.Stat(file)
		if statErr != nil {
			continue
		}
		isCompressed := strings.HasSuffix(file, ".tar.gz")
		backups = append(backups, backupFileEntry{Path: file, ModTime: info.ModTime(), IsCompressed: isCompressed})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModTime.After(backups[j].ModTime)
	})

	if l.config.CompressBackups {
		for i := len(backups) - 1; i >= 0; i-- {
			entry := backups[i]
			if !entry.IsCompressed && entry.Path != backupPathJustRotated {
				// 如果超过保留数量 且不是最新的 考虑压缩
				if len(backups) > l.config.MaxBackups && i < (len(backups)-l.config.MaxBackups) {
					// 异步压缩
					go func(path string) {
						if err := compressLogFile(path); err == nil {
							_ = os.Remove(path)
						}
					}(entry.Path)
				}
			}
		}
	}

	// 重新获取列表进行清理
	files, _ = filepath.Glob(globPattern)
	var currentBackups []backupFileEntry
	for _, file := range files {
		if filepath.Base(file) == filepath.Base(l.config.FilePath) {
			continue
		}
		info, err := os.Stat(file)
		if err == nil {
			currentBackups = append(currentBackups, backupFileEntry{Path: file, ModTime: info.ModTime()})
		}
	}
	sort.Slice(currentBackups, func(i, j int) bool {
		return currentBackups[i].ModTime.Before(currentBackups[j].ModTime)
	})

	if l.config.MaxBackups >= 0 && len(currentBackups) > l.config.MaxBackups {
		numToDelete := len(currentBackups) - l.config.MaxBackups
		for i := 0; i < numToDelete; i++ {
			_ = os.Remove(currentBackups[i].Path)
		}
	}
}

func compressLogFile(srcPath string) error {
	dstPath := srcPath + ".tar.gz"
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}

	success := false
	defer func() {
		dstFile.Close()
		if !success {
			_ = os.Remove(dstPath)
		}
	}()

	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}
	header := &tar.Header{
		Name:    filepath.Base(srcPath),
		Size:    info.Size(),
		Mode:    int64(info.Mode().Perm()),
		ModTime: info.ModTime(),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	if _, err := io.Copy(tarWriter, srcFile); err != nil {
		return err
	}
	success = true
	return nil
}
