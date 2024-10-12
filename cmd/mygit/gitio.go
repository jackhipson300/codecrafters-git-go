package main

import (
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
)

func createGitObject(objType string, contents []byte) (object []byte, hash [20]byte) {
	header := []byte(fmt.Sprintf("%s %d\x00", objType, len(contents)))
	object = append(header, contents...)

	hash = sha1.Sum(object)
	return
}

func writeGitObject(object []byte, hash [20]byte) (err error) {
	hashStr := hex.EncodeToString(hash[:])
	objectDir := fmt.Sprintf(".git/objects/%s", hashStr[:2])
	if err = os.MkdirAll(objectDir, 0755); err != nil {
		return
	}

	objectFilename := fmt.Sprintf(".git/objects/%s/%s", hashStr[:2], hashStr[2:])
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

func createAndWriteGitObject(objType string, contents []byte) (err error) {
	object, hash := createGitObject(objType, contents)
	err = writeGitObject(object, hash)
	return
}
