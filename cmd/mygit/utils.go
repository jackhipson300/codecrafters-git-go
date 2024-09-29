package main

import (
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func contentsToGitObject(contents []byte, objType string) (object []byte) {
	header := fmt.Sprintf("%s %d\x00", objType, len(contents))
	object = append([]byte(header), contents...)
	return
}

func fileToBlob(filename string) (blob []byte, hash [20]byte, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	contents, err := io.ReadAll(file)
	if err != nil {
		return
	}

	blob = contentsToGitObject(contents, "blob")
	hash = sha1.Sum(blob)

	return
}

func writeGitObject(contents []byte, hash [20]byte) error {
	hashStr := hex.EncodeToString(hash[:])
	objectDir := fmt.Sprintf(".git/objects/%s", hashStr[:2])
	if err := os.MkdirAll(objectDir, 0755); err != nil {
		return fmt.Errorf("error creating object file directory: %w", err)
	}

	objectFilename := fmt.Sprintf(".git/objects/%s/%s", hashStr[:2], hashStr[2:])
	objectFile, err := os.Create(objectFilename)
	if err != nil {
		return fmt.Errorf("error creating object file: %w", err)
	}
	defer objectFile.Close()

	zlibWriter := zlib.NewWriter(objectFile)
	defer zlibWriter.Close()
	if _, err := zlibWriter.Write(contents); err != nil {
		return fmt.Errorf("error writing to object file: %w", err)
	}

	return nil
}

func writeTreeRecursive(dir string) ([20]byte, error) {
	var rawHash [20]byte

	entries, err := os.ReadDir(dir)
	if err != nil {
		return rawHash, fmt.Errorf("error reading directory: %w", err)
	}

	output := []byte{}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}

		var hash [20]byte
		var mode string
		entryName := dir + "/" + entry.Name()
		if entry.IsDir() {
			mode = "40000"
			hash, err = writeTreeRecursive(entryName)
			if err != nil {
				return rawHash, err
			}
		} else {
			mode = "100644"
			var contents []byte
			contents, hash, err = fileToBlob(entryName)
			if err != nil {
				return rawHash, err
			}
			if err := writeGitObject(contents, hash); err != nil {
				return rawHash, err
			}
		}
		output = append(output, []byte(fmt.Sprintf("%s %s\x00", mode, entry.Name()))...)
		output = append(output, hash[:]...)
	}

	header := []byte(fmt.Sprintf("tree %d\x00", len(output)))
	tree := append(header, output...)

	rawHash = sha1.Sum(tree)

	if err := writeGitObject(tree, rawHash); err != nil {
		return rawHash, err
	}

	return rawHash, err
}
