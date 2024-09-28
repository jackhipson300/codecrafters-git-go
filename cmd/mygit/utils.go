package main

import (
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

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

	header := fmt.Sprintf("blob %d\x00", len(contents))
	blob = append([]byte(header), contents...)

	hash = sha1.Sum(blob)

	return
}

func writeBlobFile(contents []byte, hash [20]byte) error {
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

		entryName := dir + "/" + entry.Name()
		if entry.IsDir() {
			hash, err := writeTreeRecursive(entryName)
			if err != nil {
				return rawHash, err
			}

			output = append(output, []byte(fmt.Sprintf("40000 %s\x00", entry.Name()))...)
			output = append(output, hash[:]...)
		} else {
			contents, hash, err := fileToBlob(entryName)
			if err != nil {
				return rawHash, err
			}
			if err := writeBlobFile(contents, hash); err != nil {
				return rawHash, err
			}
			output = append(output, []byte(fmt.Sprintf("100644 %s\x00", entry.Name()))...)
			output = append(output, hash[:]...)
		}
	}

	header := []byte(fmt.Sprintf("tree %d\x00", len(output)))
	tree := append(header, output...)

	rawHash = sha1.Sum(tree)
	hash := hex.EncodeToString(rawHash[:])

	outputDir := fmt.Sprintf(".git/objects/%s", hash[:2])
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return rawHash, fmt.Errorf("error writing tree to file: %w", err)
	}

	outputFilename := fmt.Sprintf("%s/%s", outputDir, hash[2:])
	outputFile, err := os.Create(outputFilename)
	if err != nil {
		return rawHash, fmt.Errorf("error creating file for tree: %w", err)
	}
	defer outputFile.Close()

	zlibWriter := zlib.NewWriter(outputFile)
	defer zlibWriter.Close()
	if _, err := zlibWriter.Write(tree); err != nil {
		return rawHash, fmt.Errorf("error writing tree to file: %w", err)
	}

	return rawHash, err
}
