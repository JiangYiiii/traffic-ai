// Package aidoc 提供 AI 文档注解系统，用于标记业务文档和核心逻辑。
//
// @ai_doc 注解规范:
//   - @ai_doc 类名/函数名: 一句话描述核心业务含义
//   - @ai_doc_flow: 标记关键业务流程的入口
//   - @ai_doc_rule: 标记业务规则/约束条件
//   - @ai_doc_edge: 标记边界条件/异常处理逻辑
//
// 使用方式: 在注释中使用上述标记，辅助 AI 和开发者快速定位业务逻辑。
// 运行 `go run pkg/aidoc/scan.go ./...` 可提取项目中所有 ai_doc 注解生成索引。
package aidoc

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Annotation struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

var Tags = []string{"@ai_doc", "@ai_doc_flow", "@ai_doc_rule", "@ai_doc_edge"}

func ScanDir(root string) ([]Annotation, error) {
	var results []Annotation
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		anns, err := scanFile(path)
		if err != nil {
			return err
		}
		results = append(results, anns...)
		return nil
	})
	return results, err
}

func scanFile(path string) ([]Annotation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var anns []Annotation
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		for _, tag := range Tags {
			if idx := strings.Index(line, tag); idx >= 0 {
				content := strings.TrimSpace(line[idx+len(tag):])
				anns = append(anns, Annotation{
					File:    path,
					Line:    lineNum,
					Tag:     tag,
					Content: content,
				})
			}
		}
	}
	return anns, scanner.Err()
}

func PrintAnnotations(anns []Annotation) {
	for _, a := range anns {
		fmt.Printf("[%s] %s:%d — %s\n", a.Tag, a.File, a.Line, a.Content)
	}
}
