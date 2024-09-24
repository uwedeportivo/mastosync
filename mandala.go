package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	skybot "github.com/danrusei/gobot-bsky"
	mdon "github.com/mattn/go-mastodon"
)

type Mandala struct {
	mClient     *mdon.Client
	skyAgent    *skybot.BskyAgent
	scriptPath  string
	mandalaPath string
	tootText    string
}

func (mandala *Mandala) Generate() error {
	cmd := exec.Command("wolframscript", "-file", mandala.scriptPath, mandala.mandalaPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("failed executing mandala script:\n%s\nerror: %v", string(out), err)
		return err
	} else {
		fmt.Printf("executed mandala script:\n%s\n", string(out))
	}
	return nil
}

func (mandala *Mandala) PostMastodon() error {
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

func (mandala *Mandala) PostBlueSky() error {
	var images []skybot.Image

	uu, err := url.Parse("file://" + filepath.Join(mandala.mandalaPath, "mandala.png"))
	if err != nil {
		return err
	}
	images = append(images, skybot.Image{
		Title: "Mandala",
		Uri:   *uu,
	})

	blobs, err := mandala.skyAgent.UploadImages(context.Background(), images...)
	if err != nil {
		return err
	}

	post, err := skybot.NewPostBuilder("").
		WithImages(blobs, images).
		Build()
	if err != nil {
		return err
	}

	_, _, err = mandala.skyAgent.PostToFeed(context.Background(), post)
	if err != nil {
		return err
	}
	return nil
}

func (mandala *Mandala) Post() error {
	err := mandala.Generate()
	if err != nil {
		return err
	}

	err = mandala.PostMastodon()
	if err != nil {
		return err
	}

	return mandala.PostBlueSky()
}
