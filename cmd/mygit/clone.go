package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func discoverRefs(url string) (refs []string, err error) {
	resp, err := http.Get(fmt.Sprintf("%s/info/refs?service=git-upload-pack", url))
	if err != nil {
		return refs, fmt.Errorf("error making http request to git server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return refs, fmt.Errorf("http request to git server failed with status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return refs, fmt.Errorf("error reading http response body: %w", err)
	}

	sanityCheckRegex, err := regexp.Compile("^[0-9a-f]{4}#")
	if err != nil {
		return refs, err
	}

	if !sanityCheckRegex.Match(body[:5]) {
		return refs, fmt.Errorf("sanity check failed (first 5 bytes = %s)", body[:5])
	}

	bodyStr := string(body)
	lines := strings.Split(bodyStr, "\n")
	allRefs := []string{}

	for i := 1; i < len(lines)-1; i++ {
		allRefs = append(allRefs, strings.Split(lines[i], " ")[0][4:])
	}

	if len(allRefs) == 0 {
		return
	}

	// first ref starts with additional 0000
	allRefs[0] = allRefs[0][4:]

	// remove duplicates
	refMap := make(map[string]int)
	for _, ref := range allRefs {
		refMap[ref] = 1
	}
	for key := range refMap {
		refs = append(refs, key)
	}

	return
}

func requestPackfile(refs []string, url string) ([]byte, error) {
	var packfile []byte

	body := ""
	for _, ref := range refs {
		refStr := fmt.Sprintf("want %s\n", ref)
		refStr = fmt.Sprintf("%04x", len(refStr)+4) + refStr
		body += refStr
	}
	body += "00000009done\n"

	reqBody := bytes.NewBuffer([]byte(body))

	resp, err := http.Post(fmt.Sprintf("%s/git-upload-pack", url), "application/x-git-upload-pack-request", reqBody)
	if err != nil {
		return packfile, fmt.Errorf("error making git-upload-pack post request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return packfile, fmt.Errorf("http post request to git server failed with status code %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return packfile, fmt.Errorf("error reading git-upload-pack post response: %w", err)
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
		return packfile, fmt.Errorf("invalid pack data")
	}

	packfile = respBody[packStartIndex:]
	return packfile, nil
}

func cloneTreeFromPackfileRecursive(packfile PackFile, tree Tree, dir string) (err error) {
	if err = os.MkdirAll(dir, 0755); err != nil {
		return
	}

	for _, entry := range tree.entries {
		path := fmt.Sprintf("%s/%s", dir, entry.name)
		switch entry._type {
		case "100644":
			blob, exists := packfile.blobs[entry.hashStr]
			if !exists {
				return fmt.Errorf("blob (%s) not found", entry.hashStr)
			}
			if err = os.WriteFile(path, blob.contents, 0644); err != nil {
				return
			}
		case "40000":
			tree, exists := packfile.trees[entry.hashStr]
			if !exists {
				return fmt.Errorf("tree (%s) not found", entry.hashStr)
			}
			if err = cloneTreeFromPackfileRecursive(packfile, tree, path); err != nil {
				return
			}
		}
	}

	return
}

func cloneFromPackfile(packfile PackFile, headCommitHash string, dir string) (err error) {
	headCommit, exists := packfile.commits[headCommitHash]
	if !exists {
		return fmt.Errorf("head commit (%s) not found", headCommitHash)
	}

	rootTree, exists := packfile.trees[headCommit.tree]
	if !exists {
		return fmt.Errorf("root tree (%s) not found", rootTree.hashStr)
	}

	err = cloneTreeFromPackfileRecursive(packfile, rootTree, dir)

	return
}

func decodeCopyInstructionSizeAndOffset(idxByte byte, reader *bytes.Reader) (offset uint32, size uint32) {
	if idxByte&0x01 > 0 {
		offsetByte, _ := reader.ReadByte()
		offset += uint32(offsetByte)
	}
	if idxByte&0x02 > 0 {
		offsetByte, _ := reader.ReadByte()
		offset += uint32(offsetByte) << 8
	}
	if idxByte&0x04 > 0 {
		offsetByte, _ := reader.ReadByte()
		offset += uint32(offsetByte) << 16
	}
	if idxByte&0x08 > 0 {
		offsetByte, _ := reader.ReadByte()
		offset += uint32(offsetByte) << 24
	}
	if idxByte&0x10 > 0 {
		sizeByte, _ := reader.ReadByte()
		size += uint32(sizeByte)
	}
	if idxByte&0x20 > 0 {
		sizeByte, _ := reader.ReadByte()
		size += uint32(sizeByte) << 8
	}
	if idxByte&0x40 > 0 {
		sizeByte, _ := reader.ReadByte()
		size += uint32(sizeByte) << 16
	}

	return
}

func resolveDeltas(packfile *PackFile) error {
	for _, delta := range packfile.deltas {
		baseObj, exists := packfile.blobs[delta.baseObjHash]
		if !exists {
			// return fmt.Errorf("base object not found %s", delta.baseObjHash)
			break
		}

		resolvedObject := []byte{}
		instructionReader := bytes.NewReader(delta.instructions)
		for {
			currByte, err := instructionReader.ReadByte()
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}

			switch currByte >> 7 {
			case DELTA_REF_INSERT_INSTRUCTION:
				numBytes := uint8(currByte)
				buffer := make([]byte, numBytes)
				instructionReader.Read(buffer)
				resolvedObject = append(resolvedObject, buffer...)
			case DELTA_REF_COPY_INSTRUCTION:
				offset, size := decodeCopyInstructionSizeAndOffset(currByte, instructionReader)
				resolvedObject = append(resolvedObject, baseObj.contents[offset:offset+size]...)
			}
		}

		_, hash := createGitObject("blob", resolvedObject)
		hashStr := hex.EncodeToString(hash[:])
		packfile.blobs[hashStr] = Blob{
			hashStr:  hashStr,
			contents: resolvedObject,
		}
	}

	return nil
}
