package main

import (
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"strings"
)

func initCommand() error {
	for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("error creating directory: %w", err)
		}
	}

	headFileContents := []byte("ref: refs/heads/main\n")
	if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	fmt.Println("Initialized git directory")

	return nil
}

func catFileCommand(blobSha string) (string, error) {
	file, err := os.Open(".git/objects/" + blobSha[0:2] + "/" + blobSha[2:])
	if err != nil {
		return "", fmt.Errorf("error opening object file: %w", err)
	}
	defer file.Close()

	zlibReader, err := zlib.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("error reading file: %w", err)
	}
	defer zlibReader.Close()

	decompressed, err := io.ReadAll(zlibReader)
	if err != nil {
		return "", fmt.Errorf("error decompressing file: %w", err)
	}

	return strings.Split(string(decompressed), "\000")[1], nil
}
