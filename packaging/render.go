package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed templates
var templates embed.FS

func main() {
	version := flag.String("version", "", "release tag, such as v1.2.3")
	checksumsPath := flag.String("checksums", "", "path to the release checksums.txt")
	output := flag.String("output", "", "directory for rendered package files")
	flag.Parse()
	if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(*version) || *checksumsPath == "" || *output == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}

	content, err := os.ReadFile(*checksumsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	checksums := make(map[string]string)
	checksumPattern := regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !checksumPattern.MatchString(fields[0]) {
			fmt.Fprintf(os.Stderr, "invalid checksum on line %d\n", lineNumber+1)
			os.Exit(1)
		}
		if _, exists := checksums[fields[1]]; exists {
			fmt.Fprintf(os.Stderr, "duplicate checksum for %s\n", fields[1])
			os.Exit(1)
		}
		checksums[fields[1]] = strings.ToLower(fields[0])
	}
	for _, asset := range []string{
		"easyvenv-linux-amd64.tar.gz",
		"easyvenv-linux-arm64.tar.gz",
		"easyvenv-darwin-amd64.tar.gz",
		"easyvenv-darwin-arm64.tar.gz",
		"easyvenv-windows-amd64.zip",
		"easyvenv-windows-arm64.zip",
	} {
		if checksums[asset] == "" {
			fmt.Fprintf(os.Stderr, "missing release checksum for %s\n", asset)
			os.Exit(1)
		}
	}

	data := map[string]string{
		"Version":         strings.TrimPrefix(*version, "v"),
		"ReleaseURL":      "https://github.com/RisPNG/easyvenv/releases/download/" + *version,
		"LinuxAMD64SHA":   checksums["easyvenv-linux-amd64.tar.gz"],
		"LinuxARM64SHA":   checksums["easyvenv-linux-arm64.tar.gz"],
		"DarwinAMD64SHA":  checksums["easyvenv-darwin-amd64.tar.gz"],
		"DarwinARM64SHA":  checksums["easyvenv-darwin-arm64.tar.gz"],
		"WindowsAMD64SHA": checksums["easyvenv-windows-amd64.zip"],
		"WindowsARM64SHA": checksums["easyvenv-windows-arm64.zip"],
	}

	var files []struct {
		name    string
		content []byte
	}
	err = fs.WalkDir(templates, "templates", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(templates, name)
		if err != nil {
			return err
		}
		parsed, err := template.New(name).Option("missingkey=error").Parse(string(body))
		if err != nil {
			return err
		}
		var rendered bytes.Buffer
		if err := parsed.Execute(&rendered, data); err != nil {
			return err
		}
		relative := strings.TrimSuffix(strings.TrimPrefix(name, "templates/"), ".tmpl")
		files = append(files, struct {
			name    string
			content []byte
		}{filepath.Join(*output, filepath.FromSlash(relative)), rendered.Bytes()})
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file.name), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(file.name, file.content, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(file.name)
	}
}
