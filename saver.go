package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jomei/notionapi"
	"github.com/mattn/go-mastodon"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	"google.golang.org/api/drive/v3"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"text/scanner"
)

var stripTagsPolicy = bluemonday.StripTagsPolicy()

func ExtractTitle(status *mastodon.Status) string {
	strippedContent := stripTagsPolicy.Sanitize(status.Content)
	var s scanner.Scanner
	s.Init(strings.NewReader(strippedContent))
	var words []string
	tok := s.Scan()
	for tok != scanner.EOF {
		words = append(words, s.TokenText())
		tok = s.Scan()
	}
	numWords := 5
	if len(words) < 5 {
		numWords = len(words)
	}
	return status.Account.Username + ": " + strings.Join(words[:numWords], " ")
}

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
	gdriveService  *drive.Service
	usegdrive      bool
	bridge         string
	parent         string
}

func reverseThread(thread []*mastodon.Status) {
	i, j := 0, len(thread)-1

	for i < j {
		thread[i], thread[j] = thread[j], thread[i]
		i, j = i+1, j-1
	}
}

func (saver *Saver) StoreImage(imageURL string, filename string) (*drive.File, error) {
	resp, err := http.Get(imageURL)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("failed to close response body", err)
		}
	}(resp.Body)
	perm := drive.Permission{
		Role: "reader",
		Type: "anyone",
	}
	df := drive.File{
		Name:     filename,
		MimeType: mime.TypeByExtension(path.Ext(filename)),
		Parents:  []string{saver.parent},
	}
	dFile, err := saver.gdriveService.Files.Create(&df).Media(resp.Body).Do()
	if err != nil {
		return nil, err
	}
	_, err = saver.gdriveService.Permissions.Create(dFile.Id, &perm).Do()
	if err != nil {
		return nil, err
	}
	dFile, err = saver.gdriveService.Files.Get(dFile.Id).Fields("webContentLink").Do()
	if err != nil {
		return nil, err
	}
	return dFile, nil
}

func (saver *Saver) Blocks(thread []*mastodon.Status) notionapi.Blocks {
	var blocks notionapi.Blocks
	blocks = append(blocks, notionapi.Heading3Block{
		BasicBlock: notionapi.BasicBlock{
			Object: notionapi.ObjectTypeBlock,
			Type:   notionapi.BlockTypeHeading3,
		},
		Heading3: notionapi.Heading{
			RichText: []notionapi.RichText{
				{
					Type: notionapi.ObjectTypeText,
					Text: &notionapi.Text{
						Content: thread[0].URL,
						Link: &notionapi.Link{
							Url: thread[0].URL,
						},
					},
				},
			},
			Color: "blue",
		},
	})
	for _, status := range thread {
		blocks = append(blocks, ConvertHtml2Blocks(status.Content)...)
		for _, ma := range status.MediaAttachments {
			remoteURL := ma.RemoteURL
			if saver.usegdrive {
				filename := path.Base(remoteURL)
				dFile, err := saver.StoreImage(remoteURL, filename)
				if err == nil {
					if len(dFile.WebContentLink) > 0 {
						wcl, err := url.Parse(dFile.WebContentLink)
						if err != nil {
							log.Println("web content url is invalid: ", err)
						} else {
							gdriveId := wcl.Query().Get("id")
							remoteURL = fmt.Sprintf("%s/%s/%s", saver.bridge, gdriveId, filename)
						}
					} else if saver.debug {
						log.Println("Google Drive file doesn't have WebContentLink")
					}
				} else if saver.debug {
					log.Println("failed to store image ", remoteURL, " to Google Drive: ", err)
				}
				if saver.debug {
					log.Println("remote URL", remoteURL)
				}
			}
			blocks = append(blocks, notionapi.ImageBlock{
				BasicBlock: notionapi.BasicBlock{
					Object: notionapi.ObjectTypeBlock,
					Type:   notionapi.BlockTypeImage,
				},
				Image: notionapi.Image{
					Type: notionapi.FileTypeExternal,
					External: &notionapi.FileObject{
						URL: remoteURL,
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

func (saver *Saver) SaveUrl(tootUrl string) error {
	srs, err := saver.mClient.Search(context.Background(), tootUrl, true)
	if err != nil {
		return err
	}
	if len(srs.Statuses) > 0 {
		return saver.Save(srs.Statuses[0].ID)
	}
	fmt.Println("toot not found")
	return nil
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
		fmt.Println("toot not found")
		return nil
	}

	if len(saver.pageTitle) == 0 {
		saver.pageTitle = ExtractTitle(thread[0])
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
		var buf bytes.Buffer
		jenc := json.NewEncoder(&buf)
		jenc.SetIndent("", "    ")
		jenc.SetEscapeHTML(false)
		err := jenc.Encode(pageCreateRequest)
		if err != nil {
			return err
		}
		fmt.Printf("page request: %s\n", buf.String())
	}
	_, err := saver.notionClient.Page.Create(context.Background(), &pageCreateRequest)
	if err != nil {
		return err
	}

	return nil
}

func (saver *Saver) SaveToot(toot string) error {
	if strings.HasPrefix(toot, "https://") {
		return saver.SaveUrl(toot)
	}
	return saver.Save(mastodon.ID(toot))
}
