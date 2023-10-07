package main

import (
	"context"
	"fmt"
	"os"
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

func (ttr *Tooter) Toot() error {
	tootText, err := os.ReadFile(ttr.tootsPath)
	if err != nil {
		return err
	}

	sxs := ttr.tokenizer.Tokenize(string(tootText))

	var tootStrs []string
	var sb strings.Builder
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

	var previousStatus *mdon.Status

	for _, tootStr := range tootStrs {
		if ttr.dryrun {
			fmt.Println("====")
			fmt.Println(tootStr)
			continue
		}
		toot := mdon.Toot{
			Status: tootStr,
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
