package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	mdon "github.com/mattn/go-mastodon"
	"github.com/neurosnap/sentences"
)

type Tooter struct {
	mClient   *mdon.Client
	dryrun    bool
	tootsPath string
	tokenizer sentences.SentenceTokenizer
}

type TootImage struct {
	pos     int
	altText string
	path    string
	mediaID mdon.ID
}

var tootSeparator = regexp.MustCompile("(?m)^===$")
var mediaMarkdown = regexp.MustCompile(`!\[(?P<AltText>[^\]]*)\]\((?P<Path>.*?)\s*(?P<Title>"(?:.*[^"])")?\s*\)`)

func (ttr *Tooter) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}

	tootsDir := filepath.Dir(ttr.tootsPath)
	return filepath.Join(tootsDir, path)
}

func (ttr *Tooter) UploadImage(path string, altText string) (mdon.ID, error) {
	imgFile, err := os.Open(ttr.ResolvePath(path))
	if err != nil {
		return "", err
	}
	defer imgFile.Close()

	imgMedia := mdon.Media{
		File:        imgFile,
		Description: altText,
	}
	attachment, err := ttr.mClient.UploadMediaFromMedia(context.Background(), &imgMedia)
	if err != nil {
		return "", err
	}
	return attachment.ID, nil
}

func (ttr *Tooter) Toot() error {
	tootBytes, err := os.ReadFile(ttr.tootsPath)
	if err != nil {
		return err
	}
	tootText := string(tootBytes)

	var tootImages []*TootImage
	var totalImageMarkdownSize int

	mediaIndices := mediaMarkdown.FindAllStringSubmatchIndex(tootText, -1)
	for _, mi := range mediaIndices {
		tootImages = append(tootImages, &TootImage{
			pos:     mi[0] - totalImageMarkdownSize,
			altText: tootText[mi[2]:mi[3]],
			path:    tootText[mi[4]:mi[5]],
		})
		totalImageMarkdownSize += mi[1] - mi[0]
	}

	if !ttr.dryrun {
		for _, tootImage := range tootImages {
			imgID, err := ttr.UploadImage(tootImage.path, tootImage.altText)
			if err != nil {
				return err
			}
			tootImage.mediaID = imgID
		}
	}

	tootTextWithoutMedia := mediaMarkdown.ReplaceAllString(tootText, "")

	var tootStrs []string
	var sb strings.Builder

	tootTextParts := tootSeparator.Split(tootTextWithoutMedia, -1)

	for _, tootTextPart := range tootTextParts {
		sxs := ttr.tokenizer.Tokenize(tootTextPart)

		for _, sx := range sxs {
			tootStr := sx.Text
			if sb.Len()+len(tootStr) >= 500 {
				tootStr = strings.TrimLeftFunc(tootStr, unicode.IsSpace)
				tootStrs = append(tootStrs, sb.String())
				sb.Reset()
			}
			sb.WriteString(tootStr)
		}
		if sb.Len() > 0 {
			tootStrs = append(tootStrs, sb.String())
		}
		sb.Reset()
	}

	if sb.Len() > 0 {
		tootStrs = append(tootStrs, sb.String())
	}

	var previousStatus *mdon.Status

	var overallIndex int
	for _, tootStr := range tootStrs {
		tootEndIndex := overallIndex + len(tootStr)

		tootStr = strings.TrimSpace(tootStr)
		var mids []mdon.ID
		var imagePaths []string
		lastAssignedIndex := -1
		for i, tootImage := range tootImages {
			if tootImage.pos < tootEndIndex {
				mids = append(mids, tootImage.mediaID)
				imagePaths = append(imagePaths, tootImage.path)
				lastAssignedIndex = i
			} else {
				break
			}
		}
		tootImages = tootImages[lastAssignedIndex+1:]

		overallIndex += len(tootStr)
		if ttr.dryrun {
			fmt.Println("toot text: ", tootStr)
			fmt.Println("toot imgs: ", imagePaths)
			continue
		}
		toot := mdon.Toot{
			Status:   tootStr,
			MediaIDs: mids,
		}

		if previousStatus != nil {
			toot.InReplyToID = previousStatus.ID
		}

		status, err := ttr.mClient.PostStatus(context.Background(), &toot)
		if err != nil {
			return err
		}

		previousStatus = status
	}

	return nil
}
