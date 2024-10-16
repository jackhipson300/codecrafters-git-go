package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
)

const OBJ_COMMIT = 1
const OBJ_TREE = 2
const OBJ_BLOB = 3
const OBJ_REF_DELTA = 7

const DELTA_REF_INSERT_INSTRUCTION = 0
const DELTA_REF_COPY_INSTRUCTION = 1

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

type Delta struct {
	baseObjHash  string
	instructions []byte
}

type PackFile struct {
	commits map[string]Commit
	trees   map[string]Tree
	blobs   map[string]Blob
	deltas  []Delta
}

func NewPackfile() PackFile {
	packfile := PackFile{
		commits: make(map[string]Commit),
		trees:   make(map[string]Tree),
		blobs:   make(map[string]Blob),
		deltas:  []Delta{},
	}
	return packfile
}

func createGitObject(objType string, contents []byte) (object []byte, hash [20]byte) {
	header := []byte(fmt.Sprintf("%s %d\x00", objType, len(contents)))
	object = append(header, contents...)

	hash = sha1.Sum(object)
	return
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

func readLengthEncodedInt(reader io.Reader) (num uint64, err error) {
	var currByte uint8
	binary.Read(reader, binary.BigEndian, &currByte)
	return readLengthEncodedIntRecursive(currByte, 0, reader)
}

func readLengthEncodedIntRecursive(currByte uint8, num uint64, reader io.Reader) (uint64, error) {
	leftShift := uint64(4)
	num += (uint64(currByte) & 0x7f) << leftShift

	leftShift += 7
	msb := currByte & 0x80

	if msb == 0 {
		return num, nil
	}

	binary.Read(reader, binary.BigEndian, &currByte)
	return readLengthEncodedIntRecursive(currByte, num, reader)
}
