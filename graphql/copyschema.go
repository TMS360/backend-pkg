package graphql

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed filters/*.graphqls
var SchemaFiles embed.FS

// CopySchemas копирует GraphQL схемы в указанную директорию
func CopySchemas(destDir string) error {
	// Создаём директорию если нет
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// Копируем все файлы
	return fs.WalkDir(SchemaFiles, "filters", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		content, err := SchemaFiles.ReadFile(path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(destDir, filepath.Base(path))
		return os.WriteFile(destPath, content, 0644)
	})
}
