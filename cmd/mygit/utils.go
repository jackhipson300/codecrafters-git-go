package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func readFile(filename string) (contents []byte, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	contents, err = io.ReadAll(file)
	if err != nil {
		return
	}

	return
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
			contents, err = readFile(entryName)
			if err != nil {
				return rawHash, err
			}
			if err := createAndWriteGitObject("blob", contents); err != nil {
				return rawHash, err
			}
		}
		output = append(output, []byte(fmt.Sprintf("%s %s\x00", mode, entry.Name()))...)
		output = append(output, hash[:]...)
	}

	if err := createAndWriteGitObject("tree", output); err != nil {
		return rawHash, err
	}

	return rawHash, err
}

func readTree(rawTree []byte) (tree Tree, err error) {
	_, treeHash := createGitObject("tree", rawTree)
	tree.hashStr = hex.EncodeToString(treeHash[:])

	reader := bufio.NewReader(bytes.NewReader(rawTree))
	for {
		_type, err := reader.ReadString(' ')
		if err == io.EOF {
			break
		}

		name, err := reader.ReadString('\x00')
		if err == io.EOF {
			break
		}

		hash := make([]byte, 20)
		_, err = reader.Read(hash)
		if err == io.EOF {
			break
		}

		tree.entries = append(tree.entries, TreeEntry{
			_type:   _type[:len(_type)-1],
			name:    name[:len(name)-1],
			hashStr: hex.EncodeToString(hash[:]),
		})
	}

	return
}

type TreeEntry struct {
	_type   string
	name    string
	hashStr string
}

type Tree struct {
	hashStr string
	entries []TreeEntry
}

type Blob struct {
	hashStr  string
	contents []byte
}

type Commit struct {
	hashStr string
	tree    string
}

type PackFile struct {
	commits map[string]Commit
	trees   map[string]Tree
	blobs   map[string]Blob
}

func NewPackfile() PackFile {
	packfile := PackFile{
		commits: make(map[string]Commit),
		trees:   make(map[string]Tree),
		blobs:   make(map[string]Blob),
	}
	return packfile
}
