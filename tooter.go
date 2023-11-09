package main

import (
	"context"
	"fmt"
	"math/rand"
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

var letters = []rune("abcdefghijklmnopqrstuvwxyz")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

var tootSeparator = regexp.MustCompile("(?m)^===$")
var mediaMarkdown = regexp.MustCompile(`!\[(?P<AltText>[^\]]*)\]\((?P<Path>.*?)\s*(?P<Title>"(?:.*[^"])")?\s*\)`)
var linkMarkdown = regexp.MustCompile(`\[(?P<Text>[^\]]*)\]\((?P<Link>.*?)\)`)
var linkPlaceholder = regexp.MustCompile(`xx(?P<Key>[a-z]+)xx`)

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

	keyToLink := make(map[string]string)

	tootTextWithoutMedia = linkMarkdown.ReplaceAllStringFunc(tootTextWithoutMedia, func(matched string) string {
		openParen := strings.Index(matched, "(")
		link := matched[openParen+1 : len(matched)-1]
		key := "xx" + randSeq(19) + "xx"
		for keyToLink[key] != "" {
			key = "xx" + randSeq(19) + "xx"
		}
		keyToLink[key] = link
		return key
	})

	var tootStrs []string
	var sb strings.Builder

	tootTextParts := tootSeparator.Split(tootTextWithoutMedia, -1)

	for _, tootTextPart := range tootTextParts {
		sxs := ttr.tokenizer.Tokenize(tootTextPart)

		for _, sx := range sxs {
			tootStr := sx.Text
			if sb.Len()+len(tootStr) >= 490 {
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
	for tootIdx, tootStr := range tootStrs {
		tootEndIndex := overallIndex + len(tootStr)
		overallIndex += len(tootStr)

		tootStr = strings.TrimSpace(tootStr)

		tootStr = linkPlaceholder.ReplaceAllStringFunc(tootStr, func(matched string) string {
			return keyToLink[matched]
		})

		tootStr = fmt.Sprintf("%d/%d\n%s", tootIdx+1, len(tootStrs), tootStr)
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
