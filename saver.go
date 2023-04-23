package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jomei/notionapi"
	"github.com/mattn/go-mastodon"
	"golang.org/x/net/html"
	"strings"
)

func ConvertHtml2Blocks(content string) notionapi.Blocks {
	var blocks notionapi.Blocks

	// we start with a first paragraph even when there are no <p> so text
	// nodes have a destination
	currentParagraph := notionapi.ParagraphBlock{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeParagraph,
		},
		Paragraph: notionapi.Paragraph{},
	}
	currentHref := ""

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil
	}
	var walker func(*html.Node)
	walker = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "p" {
			// if the current paragraph has no content, we don't open a new
			// paragraph
			if len(currentParagraph.Paragraph.RichText) > 0 {
				blocks = append(blocks, currentParagraph)
				currentParagraph = notionapi.ParagraphBlock{
					BasicBlock: notionapi.BasicBlock{
						Object: notionapi.ObjectTypeBlock,
						Type:   notionapi.BlockTypeParagraph,
					},
					Paragraph: notionapi.Paragraph{},
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walker(child)
			}
		} else if node.Type == html.TextNode {
			rt := notionapi.RichText{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{
					Content: node.Data,
				},
				PlainText: node.Data,
			}
			if currentHref != "" {
				rt.Text.Link = &notionapi.Link{
					Url: currentHref,
				}
			}
			currentParagraph.Paragraph.RichText = append(currentParagraph.Paragraph.RichText, rt)
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walker(child)
			}
		} else if node.Type == html.ElementNode && node.Data == "a" {
			for _, attr := range node.Attr {
				if attr.Key == "href" {
					currentHref = attr.Val
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walker(child)
			}
			currentHref = ""
		} else if node.Type == html.ElementNode && node.Data == "span" {
			klass := ""
			for _, attr := range node.Attr {
				if attr.Key == "class" {
					klass = attr.Val
				}
			}
			if klass != "invisible" {
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					walker(child)
				}
			}
		} else {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walker(child)
			}
		}
	}
	walker(doc)

	// append the last paragraph if it has content
	if len(currentParagraph.Paragraph.RichText) > 0 {
		blocks = append(blocks, currentParagraph)
	}

	return blocks
}

type Saver struct {
	mClient        *mastodon.Client
	dryrun         bool
	notionClient   *notionapi.Client
	notionParentID string
	pageTitle      string
	debug          bool
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
		blocks = append(blocks, ConvertHtml2Blocks(status.Content)...)
		for _, ma := range status.MediaAttachments {
			blocks = append(blocks, notionapi.ImageBlock{
				BasicBlock: notionapi.BasicBlock{
					Object: notionapi.ObjectTypeBlock,
					Type:   notionapi.BlockTypeImage,
				},
				Image: notionapi.Image{
					Type: notionapi.FileTypeExternal,
					External: &notionapi.FileObject{
						URL: ma.RemoteURL,
					},
				},
			})
		}
		blocks = append(blocks, notionapi.DividerBlock{
			BasicBlock: notionapi.BasicBlock{
				Object: notionapi.ObjectTypeBlock,
				Type:   notionapi.BlockTypeDivider,
			},
			Divider: notionapi.Divider{},
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
		Content: saver.pageTitle,
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
	if saver.debug {
		pageDebug, err := json.MarshalIndent(pageCreateRequest, "", "    ")
		if err != nil {
			return err
		}
		fmt.Printf("page request: %s\n", pageDebug)
	}
	_, err := saver.notionClient.Page.Create(context.Background(), &pageCreateRequest)
	if err != nil {
		return err
	}

	return nil
}
