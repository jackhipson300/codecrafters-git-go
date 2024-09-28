package main

import (
	"bufio"
	"compress/zlib"
	"encoding/hex"
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
	file, err := os.Open(".git/objects/" + blobSha[:2] + "/" + blobSha[2:])
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

	return strings.Split(string(decompressed), "\x00")[1], nil
}

func hashFileCommand(filename string, write bool) (string, error) {
	contents, hash, err := fileToBlob(filename)
	if err != nil {
		return "", fmt.Errorf("error computing file hash: %w", err)
	}

	if write {
		writeBlobFile(contents, hash)
	}

	hashStr := hex.EncodeToString(hash[:])

	return hashStr, nil
}

func lsTreeCommand(treeSha string) (string, error) {
	treeFilename := fmt.Sprintf(".git/objects/%s/%s", treeSha[:2], treeSha[2:])
	treeFile, err := os.Open(treeFilename)
	if err != nil {
		return "", fmt.Errorf("error opening tree file: %w", err)
	}
	defer treeFile.Close()

	zlibReader, err := zlib.NewReader(treeFile)
	if err != nil {
		return "", fmt.Errorf("error reading tree file: %w", err)
	}
	defer zlibReader.Close()

	output := ""

	reader := bufio.NewReader(zlibReader)
	reader.ReadBytes('\x00')
	for {
		if _, err := reader.ReadString(' '); err == io.EOF {
			break
		}

		filename, err := reader.ReadString('\x00')
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("error parsing tree: %w", err)
		}

		output += filename[:len(filename)-1] + "\n"
	}

	return output, nil
}

func writeTreeCommand(dir string) (hash string, err error) {
	rawHash, err := writeTreeRecursive(dir)
	hash = hex.EncodeToString(rawHash[:])
	return
}
