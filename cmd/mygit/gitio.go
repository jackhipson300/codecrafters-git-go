package main

import (
	"compress/zlib"
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

func writeTreeRecursive(dir string) (rawHash [20]byte, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
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
				return
			}
		} else {
			mode = "100644"
			var contents []byte
			var blob []byte
			contents, err = readFile(entryName)
			if err != nil {
				return
			}

			blob, hash = createGitObject("blob", contents)
			if err = writeGitObject(blob, hash); err != nil {
				return
			}
		}
		output = append(output, []byte(fmt.Sprintf("%s %s\x00", mode, entry.Name()))...)
		output = append(output, hash[:]...)
	}

	rawObj, rawHash := createGitObject("tree", output)
	if err != nil {
		return
	}

	err = writeGitObject(rawObj, rawHash)
	return
}

func writeGitObjectToDir(object []byte, hash [20]byte, dir string) (err error) {
	hashStr := hex.EncodeToString(hash[:])
	objectDir := fmt.Sprintf("%s.git/objects/%s", dir, hashStr[:2])
	if err = os.MkdirAll(objectDir, 0755); err != nil {
		return
	}

	objectFilename := fmt.Sprintf("%s.git/objects/%s/%s", dir, hashStr[:2], hashStr[2:])
	objectFile, err := os.Create(objectFilename)
	if err != nil {
		return
	}
	defer objectFile.Close()

	zlibWriter := zlib.NewWriter(objectFile)
	defer zlibWriter.Close()
	if _, err = zlibWriter.Write(object); err != nil {
		return
	}

	return
}

func writeGitObject(object []byte, hash [20]byte) (err error) {
	return writeGitObjectToDir(object, hash, "")
}

func createAndWriteGitObjectToDir(objType string, contents []byte, dir string) (err error) {
	object, hash := createGitObject(objType, contents)
	err = writeGitObjectToDir(object, hash, dir)
	return
}
