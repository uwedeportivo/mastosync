package main

import (
	"context"
	"fmt"
	"github.com/mattn/go-mastodon"
	"os"
	"os/exec"
)

type Mandala struct {
	mClient    *mastodon.Client
	scriptPath string
}

func (mandala *Mandala) Post() error {
	cmd := exec.Command("wolframscript", "-file", mandala.scriptPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("failed executing mandala script:\n%s\nerror: %v", string(out), err)
		return err
	}
	mandalaFile, err := os.Open("/tmp/mandala.png")
	if err != nil {
		return err
	}
	defer mandalaFile.Close()
	mandalaMedia := mastodon.Media{
		File:        mandalaFile,
		Description: "Colorful Mandala generated with https://mathematica.stackexchange.com/q/136974",
	}
	attachment, err := mandala.mClient.UploadMediaFromMedia(context.Background(), &mandalaMedia)
	if err != nil {
		return err
	}
	toot := mastodon.Toot{
		Status:   "#Mondala",
		MediaIDs: []mastodon.ID{attachment.ID},
	}

	_, err = mandala.mClient.PostStatus(context.Background(), &toot)
	if err != nil {
		return err
	}
	return nil
}
