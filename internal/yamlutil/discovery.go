package yamlutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Discover(positionalPaths []string, fileFlags []string) ([]string, error) {
	inputs := append([]string{}, positionalPaths...)
	inputs = append(inputs, fileFlags...)
	if len(inputs) == 0 {
		inputs = []string{"."}
	}

	seen := map[string]struct{}{}
	var files []string
	for _, input := range inputs {
		discovered, err := discoverOne(input)
		if err != nil {
			return nil, err
		}
		for _, file := range discovered {
			clean := filepath.Clean(file)
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			files = append(files, clean)
		}
	}
	sort.Strings(files)
	return files, nil
}

func discoverOne(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.IsDir() {
		if !isYAML(path) {
			return nil, fmt.Errorf("%s is not a .yaml or .yml file", path)
		}
		return []string{path}, nil
	}

	var files []string
	err = filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if current != path && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if isYAML(current) {
			files = append(files, current)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return files, nil
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}
