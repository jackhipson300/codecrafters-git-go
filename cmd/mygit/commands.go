package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
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
	refs, err := discoverRefs(url)
	if err != nil {
		return "", fmt.Errorf("error discovering refs: %w", err)
	}

	body := ""
	for _, ref := range refs {
		fmt.Println("ref", ref)
		refStr := fmt.Sprintf("want %s\n", ref)
		refStr = fmt.Sprintf("%04x", len(refStr)+4) + refStr
		body += refStr
	}
	body += "00000009done\n"

	reqBody := bytes.NewBuffer([]byte(body))

	resp, err := http.Post(fmt.Sprintf("%s/git-upload-pack", url), "application/x-git-upload-pack-request", reqBody)
	if err != nil {
		return "", fmt.Errorf("error making git-upload-pack post request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("http post request to git server failed with status code %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading git-upload-pack post response: %w", err)
	}

	packStartIndex := -1
	for i, b := range respBody {
		if b == 'P' {
			if string(respBody[i:i+4]) == "PACK" {
				packStartIndex = i + 4
			}
		}
	}

	if packStartIndex == -1 {
		return "", fmt.Errorf("invalid pack data")
	}

	pack := respBody[packStartIndex:]

	header := pack[:8]
	compressedObjects := pack[8:]

	headerReader := bytes.NewReader(header)
	var version, numObjects uint32
	if err := binary.Read(headerReader, binary.BigEndian, &version); err != nil {
		return "", err
	}
	if err := binary.Read(headerReader, binary.BigEndian, &numObjects); err != nil {
		return "", err
	}

	if version != 2 {
		return "", fmt.Errorf("unsupported version: %d", version)
	}

	packfile := NewPackfile()
	objectsReader := bytes.NewReader(compressedObjects)
	for i := uint32(0); i < numObjects; i++ {
		var currByte uint8
		binary.Read(objectsReader, binary.BigEndian, &currByte)
		objType := (currByte & 0x70) >> 4

		size := uint64(currByte & 0x0f)
		msb := currByte & 0x80

		leftShift := uint64(4)
		for msb > 0 {
			binary.Read(objectsReader, binary.BigEndian, &currByte)
			size += (uint64(currByte) & 0x7f) << leftShift

			leftShift += 7
			msb = currByte & 0x80
		}

		if objType == 7 {
			baseObjName := make([]byte, 20)
			objectsReader.Read(baseObjName)
			fmt.Printf("base obj: %x\n", baseObjName)
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
		case 1:
			_, hash := createGitObject("commit", rawObj)
			hashStr := hex.EncodeToString(hash[:])
			treeHeader := strings.Split(string(rawObj), "\n")[0]
			treeHash := strings.Split(treeHeader, " ")[1]
			packfile.commits[hashStr] = Commit{
				hashStr: hashStr,
				tree:    treeHash,
			}
		case 2:
			tree, err := readTree(rawObj)
			if err != nil {
				return "", fmt.Errorf("error parsing tree: %w", err)
			}
			packfile.trees[tree.hashStr] = tree
		case 3:
			bytes, hash := createGitObject("blob", rawObj)
			hashStr := hex.EncodeToString(hash[:])
			packfile.blobs[hashStr] = Blob{
				hashStr:  hashStr,
				contents: rawObj,
			}
			fmt.Println(string(bytes))
			fmt.Printf("%x\n", bytes)
		}
	}

	if err := cloneFromPackfile(packfile, refs[0]); err != nil {
		fmt.Println("err", err.Error())
	}

	return "", nil
}
