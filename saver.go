package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"text/scanner"

	"github.com/jomei/notionapi"
	mdon "github.com/mattn/go-mastodon"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	googdrive "google.golang.org/api/drive/v3"
)

var stripTagsPolicy = bluemonday.StripTagsPolicy()

func ExtractTitle(status *mdon.Status) string {
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

type InternalFileObject struct {
	Type     string `json:"type"`
	FileID   string `json:"file_id,omitempty"`
	URL      string `json:"url,omitempty"`
}

type InternalImage struct {
	Type     string              `json:"type"`
	File     *InternalFileObject `json:"file,omitempty"`
	External *InternalFileObject `json:"external,omitempty"`
}

type InternalImageBlock struct {
	notionapi.BasicBlock
	Image InternalImage `json:"image"`
}

type Saver struct {
	mClient        *mdon.Client
	dryrun         bool
	notionClient   *notionapi.Client
	notionToken    string
	notionParentID string
	pageTitle      string
	debug          bool
	gdriveService  *googdrive.Service
	usegdrive      bool
	bridge         string
	parent         string
}

func reverseThread(thread []*mdon.Status) {
	i, j := 0, len(thread)-1

	for i < j {
		thread[i], thread[j] = thread[j], thread[i]
		i, j = i+1, j-1
	}
}

func (saver *Saver) UploadToNotion(imageURL string, filename string) (string, error) {
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 1. Create file upload object
	uploadReqBody := map[string]string{
		"name":         filename,
		"content_type": mime.TypeByExtension(path.Ext(filename)),
	}
	reqJSON, _ := json.Marshal(uploadReqBody)
	req, err := http.NewRequest("POST", "https://api.notion.com/v1/file_uploads", bytes.NewBuffer(reqJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+saver.notionToken)
	req.Header.Set("Notion-Version", "2022-06-28") // Or whatever version is required for this endpoint
	req.Header.Set("Content-Type", "application/json")

	notionResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer notionResp.Body.Close()

	if notionResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(notionResp.Body)
		return "", fmt.Errorf("failed to create file upload: %s", string(body))
	}

	var uploadData struct {
		ID        string `json:"id"`
		UploadURL string `json:"upload_url"`
	}
	if err := json.NewDecoder(notionResp.Body).Decode(&uploadData); err != nil {
		return "", err
	}

	// 2. Upload file content
	// Re-fetch body or use a buffer if the file is small. 
	// For now, let's read the whole thing into a buffer to be safe for Re-requesting if needed, 
	// but we already have resp.Body. Since we closed it, we need to Re-fetch or use a buffer.
	// Actually, let's read it into a buffer first.
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	uploadReq, err := http.NewRequest("PUT", uploadData.UploadURL, bytes.NewBuffer(content))
	if err != nil {
		return "", err
	}
	// Notion's documentation says to use the upload_url directly.
	// Usually for S3/GCS signed URLs, we don't need many headers.
	uploadReq.Header.Set("Content-Type", uploadReqBody["content_type"])

	uploadResp, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		return "", err
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(uploadResp.Body)
		return "", fmt.Errorf("failed to upload file content: %s", string(body))
	}

	return uploadData.ID, nil
}

func (saver *Saver) StoreImage(imageURL string, filename string) (*googdrive.File, error) {
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
	perm := googdrive.Permission{
		Role: "reader",
		Type: "anyone",
	}
	df := googdrive.File{
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

func (saver *Saver) Blocks(thread []*mdon.Status) notionapi.Blocks {
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
			var fileID string
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
			} else {
				filename := path.Base(remoteURL)
				id, err := saver.UploadToNotion(remoteURL, filename)
				if err == nil {
					fileID = id
				} else if saver.debug {
					log.Println("failed to upload image to Notion: ", err)
				}
			}

			if fileID != "" {
				blocks = append(blocks, InternalImageBlock{
					BasicBlock: notionapi.BasicBlock{
						Object: notionapi.ObjectTypeBlock,
						Type:   notionapi.BlockTypeImage,
					},
					Image: InternalImage{
						Type: "file",
						File: &InternalFileObject{
							Type:   "file",
							FileID: fileID,
						},
					},
				})
			} else {
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

func (saver *Saver) Save(id mdon.ID) error {
	var thread []*mdon.Status

	if saver.notionToken == "" && saver.notionClient != nil {
		saver.notionToken = string(saver.notionClient.Token)
	}

	for id != "" {
		status, err := saver.mClient.GetStatus(context.Background(), id)
		if err != nil {
			return err
		}
		thread = append(thread, status)
		if status.InReplyToID != nil {
			prevId := status.InReplyToID.(string)
			id = mdon.ID(prevId)
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

	blocks := saver.Blocks(thread)

	for i := 0; i <= len(blocks)/100; i++ {
		titleStr := saver.pageTitle
		if i > 0 {
			titleStr = fmt.Sprintf("%s %d", saver.pageTitle, i)
		}
		title := notionapi.Text{
			Content: titleStr,
		}
		pageBlocks := blocks[i*100 : min(len(blocks), (i+1)*100)]

		if len(pageBlocks) == 0 {
			break
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
			Children: pageBlocks,
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
	}

	return nil
}

func (saver *Saver) SaveToot(toot string) error {
	if strings.HasPrefix(toot, "https://") {
		return saver.SaveUrl(toot)
	}
	return saver.Save(mdon.ID(toot))
}
