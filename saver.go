package main

import (
	"context"
	"github.com/jomei/notionapi"
	"github.com/mattn/go-mastodon"
)

type Saver struct {
	mClient        *mastodon.Client
	dryrun         bool
	notionClient   *notionapi.Client
	notionParentID string
}

func reverseThread(thread []*mastodon.Status) {
	i, j := 0, len(thread)-1

	for i < j {
		thread[i], thread[j] = thread[j], thread[i]
		i, j = i+1, j-1
	}
}

func (saver *Saver) Blocks(thread []*mastodon.Status) notionapi.Blocks {
	var blocks notionapi.Blocks
	for _, status := range thread {
		txt := notionapi.Text{
			Content: status.Content,
		}
		blocks = append(blocks, notionapi.ParagraphBlock{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeParagraph,
			},
			Paragraph: notionapi.Paragraph{
				RichText: []notionapi.RichText{
					{
						Type: notionapi.ObjectTypeText,
						Text: &txt,
					},
				},
			},
		})
	}
	return blocks
}

func (saver *Saver) Save(id mastodon.ID) error {
	var thread []*mastodon.Status

	for id != "" {
		status, err := saver.mClient.GetStatus(context.Background(), id)
		if err != nil {
			return err
		}
		thread = append(thread, status)
		if status.InReplyToID != nil {
			prevId := status.InReplyToID.(string)
			id = mastodon.ID(prevId)
		} else {
			id = ""
		}
	}
	reverseThread(thread)

	if len(thread) == 0 {
		return nil
	}

	title := notionapi.Text{
		Content: thread[0].URL,
	}

	pageCreateRequest := notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:   notionapi.ParentTypePageID,
			PageID: notionapi.PageID(saver.notionParentID),
		},
		Properties: notionapi.Properties{
			"title": notionapi.TitleProperty{
				Title: []notionapi.RichText{
					{
						Type: notionapi.ObjectTypeText,
						Text: &title,
					},
				},
			},
		},
		Children: saver.Blocks(thread),
	}
	_, err := saver.notionClient.Page.Create(context.Background(), &pageCreateRequest)
	if err != nil {
		return err
	}

	return nil
}
