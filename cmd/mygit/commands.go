package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func initCommand(dir string) error {
	for _, currDir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(fmt.Sprint(dir, currDir), 0755); err != nil {
			return fmt.Errorf("error creating directory: %w", err)
		}
	}

	headFileContents := []byte("ref: refs/heads/main\n")
	if err := os.WriteFile(fmt.Sprint(dir, ".git/HEAD"), headFileContents, 0644); err != nil {
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
	contents, err := readFile(filename)
	if err != nil {
		return "", fmt.Errorf("error reading file: %w", err)
	}

	object, hash := createGitObject("blob", contents)
	if write {
		writeGitObject(object, hash)
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

	rawTree, err := io.ReadAll(zlibReader)
	if err != nil {
		return "", fmt.Errorf("error decompressing tree file: %w", err)
	}

	nullByteIndex := 0
	for i, b := range rawTree {
		if b == '\x00' {
			nullByteIndex = i
			break
		}
	}

	tree, err := readTree(rawTree[nullByteIndex:])
	if err != nil {
		return "", fmt.Errorf("error parsing tree: %w", err)
	}

	for _, entry := range tree.entries {
		output += entry.name + "\n"
	}

	return output, nil
}

func writeTreeCommand(dir string) (hash string, err error) {
	rawHash, err := writeTreeRecursive(dir)
	hash = hex.EncodeToString(rawHash[:])
	return
}

func commitTreeCommand(
	treeSha string,
	commitSha string,
	message string,
	authorName string,
	authorEmail string,
	timestamp int64,
	timezone string,
) (string, error) {
	commit := fmt.Sprintf("tree %s\nparent %s\nauthor %s <%s> %d %s\ncommitter %s <%s> %d %s\n\n%s\n",
		treeSha,
		commitSha,
		authorName,
		authorEmail,
		timestamp,
		timezone,
		authorName,
		authorEmail,
		timestamp,
		timezone,
		message,
	)
	commitObj, hash := createGitObject("commit", []byte(commit))

	if err := writeGitObject(commitObj, hash); err != nil {
		return "", err
	}

	hashStr := hex.EncodeToString(hash[:])

	return hashStr, nil
}

func cloneCommand(url string, dir string) (string, error) {
	initCommand(dir)

	refs, err := discoverRefs(url)
	if err != nil {
		return "", fmt.Errorf("error discovering refs: %w", err)
	}

	rawPackfile, err := requestPackfile(refs, url)
	if err != nil {
		return "", fmt.Errorf("error getting packfile: %w", err)
	}

	header := rawPackfile[:8]
	objects := rawPackfile[8:]

	headerReader := bytes.NewReader(header)
	var version, numObjects uint32
	if err := binary.Read(headerReader, binary.BigEndian, &version); err != nil {
		return "", fmt.Errorf("error parsing packfile: %w", err)
	}
	if err := binary.Read(headerReader, binary.BigEndian, &numObjects); err != nil {
		return "", fmt.Errorf("error parsing packfile: %w", err)
	}

	if version != 2 {
		return "", fmt.Errorf("unsupported version: %d", version)
	}

	packfile := NewPackfile()
	objectsReader := bytes.NewReader(objects)
	for i := uint32(0); i < numObjects; i++ {
		var currByte uint8
		binary.Read(objectsReader, binary.BigEndian, &currByte)
		objType := (currByte & 0x70) >> 4
		readLengthEncodedIntRecursive(currByte&0x8f, 0, objectsReader)

		baseObjHash := make([]byte, 20)
		if objType == 7 {
			objectsReader.Read(baseObjHash)
		}

		zlibReader, err := zlib.NewReader(objectsReader)
		if err != nil {
			return "", fmt.Errorf("error creating reader for compressed object: %w", err)
		}
		defer zlibReader.Close()

		rawObj, err := io.ReadAll(zlibReader)
		if err != nil {
			return "", fmt.Errorf("error decompressing object: %w", err)
		}

		switch objType {
		case OBJ_COMMIT:
			commitObj, hash := createGitObject("commit", rawObj)
			writeGitObjectToDir(commitObj, hash, dir)
			hashStr := hex.EncodeToString(hash[:])
			treeHeader := strings.Split(string(rawObj), "\n")[0]
			treeHash := strings.Split(treeHeader, " ")[1]
			packfile.commits[hashStr] = Commit{
				hashStr: hashStr,
				tree:    treeHash,
			}
		case OBJ_TREE:
			tree, err := readTree(rawObj)
			if err != nil {
				return "", fmt.Errorf("error parsing tree: %w", err)
			}

			createAndWriteGitObjectToDir("tree", rawObj, dir)
			packfile.trees[tree.hashStr] = tree
		case OBJ_BLOB:
			blobObj, hash := createGitObject("blob", rawObj)
			writeGitObjectToDir(blobObj, hash, dir)
			hashStr := hex.EncodeToString(hash[:])
			packfile.blobs[hashStr] = Blob{
				hashStr:  hashStr,
				contents: rawObj,
			}
		case OBJ_REF_DELTA:
			deltaReader := bytes.NewReader(rawObj)
			/* The first section of the decompressed delta data are the base and result object
			   sizes. We don't really need them to resolve deltas so we just ignore them here. */
			readLengthEncodedInt(deltaReader)
			readLengthEncodedInt(deltaReader)

			instructions, _ := io.ReadAll(deltaReader)

			packfile.deltas = append(packfile.deltas, Delta{
				baseObjHash:  hex.EncodeToString(baseObjHash[:]),
				instructions: instructions,
			})
		}
	}

	if err := resolveDeltas(&packfile); err != nil {
		fmt.Println("err", err.Error())
	}

	if err := cloneFromPackfile(packfile, refs[0], dir); err != nil {
		fmt.Println("err", err.Error())
	}

	return "", nil
}
