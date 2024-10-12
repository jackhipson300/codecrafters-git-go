package main

import (
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
	fmt.Println(bodyStr)
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

func cloneTreeFromPackfileRecursive(packfile PackFile, tree Tree, dir string) (err error) {
	fmt.Println("dir", dir)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return
	}

	for _, entry := range tree.entries {
		path := fmt.Sprintf("%s/%s", dir, entry.name)
		fmt.Println("entry", entry._type, path)
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

func cloneFromPackfile(packfile PackFile, headCommitHash string) (err error) {
	headCommit, exists := packfile.commits[headCommitHash]
	if !exists {
		return fmt.Errorf("head commit (%s) not found", headCommitHash)
	}

	rootTree, exists := packfile.trees[headCommit.tree]
	if !exists {
		return fmt.Errorf("root tree (%s) not found", rootTree.hashStr)
	}

	err = cloneTreeFromPackfileRecursive(packfile, rootTree, "test")

	// for _, entry := range rootTree.entries {
	// 	fmt.Println("entry", entry.name, entry.hashStr)
	// }

	// for _, blob := range packfile.blobs {
	// 	fmt.Println("entry", blob.hashStr, string(blob.contents))
	// 	fmt.Printf("%x\n", blob.contents)
	// }

	return
}
