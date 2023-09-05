package main

import (
	"context"
	"fmt"
	"github.com/mattn/go-mastodon"
	"os"
	"os/exec"
	"path/filepath"
)

type Mandala struct {
	mClient     *mastodon.Client
	scriptPath  string
	mandalaPath string
	choose      string
	tootText    string
}

func (mandala *Mandala) Post() error {
	if mandala.choose == "" {
		cmd := exec.Command("wolframscript", "-file", mandala.scriptPath, mandala.mandalaPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("failed executing mandala script:\n%s\nerror: %v", string(out), err)
			return err
		} else {
			fmt.Printf("executed mandala script:\n%s\n", string(out))
		}
	} else {
		mandalaFile, err := os.Open(filepath.Join(mandala.mandalaPath, mandala.choose))
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

		if mandala.tootText != "" {
			toot.Status += " " + mandala.tootText
		}

		_, err = mandala.mClient.PostStatus(context.Background(), &toot)
		if err != nil {
			return err
		}
	}
	return nil
}
