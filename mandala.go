package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	mdon "github.com/mattn/go-mastodon"
)

type Mandala struct {
	mClient     *mdon.Client
	scriptPath  string
	mandalaPath string
	tootText    string
}

func (mandala *Mandala) Post() error {
	cmd := exec.Command("wolframscript", "-file", mandala.scriptPath, mandala.mandalaPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("failed executing mandala script:\n%s\nerror: %v", string(out), err)
		return err
	} else {
		fmt.Printf("executed mandala script:\n%s\n", string(out))
	}

	mandalaFile, err := os.Open(filepath.Join(mandala.mandalaPath, "mandala.png"))
	if err != nil {
		return err
	}
	defer mandalaFile.Close()
	mandalaMedia := mdon.Media{
		File:        mandalaFile,
		Description: "Colorful Mandala generated with https://mathematica.stackexchange.com/q/136974",
	}
	attachment, err := mandala.mClient.UploadMediaFromMedia(context.Background(), &mandalaMedia)
	if err != nil {
		return err
	}
	toot := mdon.Toot{
		Status:   "#Mondala",
		MediaIDs: []mdon.ID{attachment.ID},
	}

	if mandala.tootText != "" {
		toot.Status += " " + mandala.tootText
	}

	_, err = mandala.mClient.PostStatus(context.Background(), &toot)
	if err != nil {
		return err
	}
	return nil
}
