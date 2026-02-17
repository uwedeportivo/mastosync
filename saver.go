package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/scanner"
	"time"

	"github.com/jomei/notionapi"
	mdon "github.com/mattn/go-mastodon"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	googdrive "google.golang.org/api/drive/v3"
	"gopkg.in/yaml.v3"

	"github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	skybot "github.com/danrusei/gobot-bsky"
)

var stripTagsPolicy = bluemonday.StripTagsPolicy()

type SavedMedia struct {
	ID        string
	URL       string
	RemoteURL string
}

type SavedStatus struct {
	ID        string
	Content   string
	URL       string
	CreatedAt time.Time
	Account   struct {
		Username    string
		DisplayName string
		Acct        string
	}
	Tags []struct {
		Name string
	}
	MediaAttachments []SavedMedia
}

func ExtractTitle(status *SavedStatus) string {
	strippedContent := stripTagsPolicy.Sanitize(status.Content)
	var s scanner.Scanner
	s.Init(strings.NewReader(strippedContent))
	var words []string
	tok := s.Scan()
	for tok != scanner.EOF {
		words = append(words, s.TokenText())
		tok = s.Scan()
	}
	numWords := min(len(words), 5)
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
	Type   string `json:"type"`
	FileID string `json:"file_id,omitempty"`
	URL    string `json:"url,omitempty"`
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

type Fetcher interface {
	Fetch(ctx context.Context, idOrUrl string) ([]*SavedStatus, error)
}

type MastodonFetcher struct {
	mClient *mdon.Client
}

func (mf *MastodonFetcher) Fetch(ctx context.Context, idOrUrl string) ([]*SavedStatus, error) {
	id := idOrUrl
	if strings.HasPrefix(idOrUrl, "https://") {
		srs, err := mf.mClient.Search(ctx, idOrUrl, true)
		if err != nil {
			return nil, err
		}
		if len(srs.Statuses) == 0 {
			return nil, fmt.Errorf("toot not found")
		}
		id = string(srs.Statuses[0].ID)
	}

	var thread []*mdon.Status
	for id != "" {
		status, err := mf.mClient.GetStatus(ctx, mdon.ID(id))
		if err != nil {
			return nil, err
		}
		thread = append(thread, status)
		if status.InReplyToID != nil {
			id = status.InReplyToID.(string)
		} else {
			id = ""
		}
	}

	// Reverse thread
	for i, j := 0, len(thread)-1; i < j; i, j = i+1, j-1 {
		thread[i], thread[j] = thread[j], thread[i]
	}

	var result []*SavedStatus
	for _, s := range thread {
		ss := &SavedStatus{
			ID:        string(s.ID),
			Content:   s.Content,
			URL:       s.URL,
			CreatedAt: s.CreatedAt,
		}
		ss.Account.Username = s.Account.Username
		ss.Account.DisplayName = s.Account.DisplayName
		ss.Account.Acct = s.Account.Acct
		for _, t := range s.Tags {
			ss.Tags = append(ss.Tags, struct{ Name string }{Name: t.Name})
		}
		for _, ma := range s.MediaAttachments {
			ss.MediaAttachments = append(ss.MediaAttachments, SavedMedia{
				ID:        string(ma.ID),
				URL:       ma.URL,
				RemoteURL: ma.RemoteURL,
			})
		}
		result = append(result, ss)
	}
	return result, nil
}

type BlueskyFetcher struct {
	skyAgent *skybot.BskyAgent
}

func (bf *BlueskyFetcher) Fetch(ctx context.Context, idOrUrl string) ([]*SavedStatus, error) {
	uri := idOrUrl
	if strings.HasPrefix(idOrUrl, "https://") {
		// Example: https://bsky.app/profile/danrusei.bsky.social/post/3j7z7z7z7z7z7
		u, err := url.Parse(idOrUrl)
		if err != nil {
			return nil, err
		}
		parts := strings.Split(u.Path, "/")
		if len(parts) < 5 {
			return nil, fmt.Errorf("invalid bluesky url")
		}
		handle := parts[2]
		postID := parts[4]

		resolve, err := atproto.IdentityResolveHandle(ctx, bf.skyAgent.Client(), handle)
		if err != nil {
			return nil, err
		}
		uri = fmt.Sprintf("at://%s/app.bsky.feed.post/%s", resolve.Did, postID)
	}

	threadOutput, err := appbsky.FeedGetPostThread(ctx, bf.skyAgent.Client(), 0, 10, uri)
	if err != nil {
		return nil, err
	}

	var thread []*appbsky.FeedDefs_ThreadViewPost
	curr := threadOutput.Thread.FeedDefs_ThreadViewPost
	for curr != nil {
		thread = append(thread, curr)
		if curr.Parent != nil && curr.Parent.FeedDefs_ThreadViewPost != nil {
			curr = curr.Parent.FeedDefs_ThreadViewPost
		} else {
			curr = nil
		}
	}

	// Reverse thread to original order
	for i, j := 0, len(thread)-1; i < j; i, j = i+1, j-1 {
		thread[i], thread[j] = thread[j], thread[i]
	}

	var result []*SavedStatus
	for _, p := range thread {
		post := p.Post
		ss := &SavedStatus{
			ID:      post.Cid,
			Content: post.Record.Val.(*appbsky.FeedPost).Text,
			URL:     fmt.Sprintf("https://bsky.app/profile/%s/post/%s", post.Author.Handle, filepath.Base(post.Uri)),
		}
		if t, err := time.Parse(time.RFC3339, post.Record.Val.(*appbsky.FeedPost).CreatedAt); err == nil {
			ss.CreatedAt = t
		}
		ss.Account.Username = post.Author.Handle
		if post.Author.DisplayName != nil {
			ss.Account.DisplayName = *post.Author.DisplayName
		}
		ss.Account.Acct = post.Author.Handle

		if post.Embed != nil && post.Embed.EmbedImages_View != nil {
			for i, img := range post.Embed.EmbedImages_View.Images {
				ss.MediaAttachments = append(ss.MediaAttachments, SavedMedia{
					ID:        fmt.Sprintf("%s-%d", post.Cid, i),
					URL:       img.Fullsize,
					RemoteURL: img.Fullsize,
				})
			}
		}
		result = append(result, ss)
	}

	return result, nil
}

type Saver struct {
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
	outputPath     string
	fetcher        Fetcher
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

func (saver *Saver) Blocks(thread []*SavedStatus) notionapi.Blocks {
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

func (saver *Saver) SaveToDirectory(thread []*SavedStatus) error {
	if saver.outputPath == "" {
		return fmt.Errorf("output path not set")
	}

	if err := os.MkdirAll(saver.outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	imagesDir := filepath.Join(saver.outputPath, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create images directory: %w", err)
	}

	if len(saver.pageTitle) == 0 {
		saver.pageTitle = ExtractTitle(thread[0])
	}

	// Sanitize filename
	filename := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, saver.pageTitle) + ".md"

	mdPath := filepath.Join(saver.outputPath, filename)

	var buf bytes.Buffer

	// Frontmatter
	type Frontmatter struct {
		Title  string    `yaml:"title"`
		Date   time.Time `yaml:"date"`
		Author string    `yaml:"author"`
		URL    string    `yaml:"url"`
		ID     string    `yaml:"id"`
		Tags   []string  `yaml:"tags"`
	}

	tags := []string{}
	if strings.Contains(thread[0].URL, "bsky.app") {
		tags = append(tags, "bluesky")
	} else {
		tags = append(tags, "mastodon")
	}

	for _, tag := range thread[0].Tags {
		tags = append(tags, tag.Name)
	}

	fm := Frontmatter{
		Title:  saver.pageTitle,
		Date:   thread[0].CreatedAt,
		Author: fmt.Sprintf("%s (@%s)", thread[0].Account.DisplayName, thread[0].Account.Acct),
		URL:    thread[0].URL,
		ID:     string(thread[0].ID),
		Tags:   tags,
	}

	buf.WriteString("---\n")
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("failed to marshal frontmatter: %w", err)
	}
	buf.Write(fmBytes)
	buf.WriteString("---\n\n")

	// Content
	for _, status := range thread {
		// Simple HTML to Markdown conversion for content
		content := status.Content
		// Strip some tags or convert them
		// This is a simplified version, ideally use a proper html-to-markdown lib
		content = strings.ReplaceAll(content, "<p>", "")
		content = strings.ReplaceAll(content, "</p>", "\n\n")
		content = strings.ReplaceAll(content, "<br />", "\n")
		content = strings.ReplaceAll(content, "<br/>", "\n")

		// Remove other tags for a cleaner look
		content = stripTagsPolicy.Sanitize(content)

		buf.WriteString(content)
		buf.WriteString("\n")

		for _, ma := range status.MediaAttachments {
			hashedName, err := saver.downloadAndHash(ma.URL, imagesDir)
			if err != nil {
				log.Printf("failed to download attachment %s: %v", ma.URL, err)
				continue
			}

			// Obsidian link to images subfolder
			buf.WriteString(fmt.Sprintf("![[images/%s]]\n", hashedName))
		}
		buf.WriteString("\n---\n\n")
	}

	return os.WriteFile(mdPath, buf.Bytes(), 0644)
}

func (saver *Saver) downloadAndHash(url string, destDir string) (string, error) {
	if saver.dryrun {
		log.Printf("[dryrun] would download and hash %s to %s", url, destDir)
		return "dryrun_hash.png", nil
	}
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download file: %s", resp.Status)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	ext := strings.ToLower(filepath.Ext(url))
	if ext == "" {
		// Try to get extension from Content-Type
		ct := resp.Header.Get("Content-Type")
		exts, _ := mime.ExtensionsByType(ct)
		if len(exts) > 0 {
			ext = exts[0]
		}
	}

	if ext == ".jfif" || resp.Header.Get("Content-Type") == "image/jfif" {
		img, _, err := image.Decode(bytes.NewReader(content))
		if err == nil {
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, img, nil); err == nil {
				content = buf.Bytes()
				ext = ".jpg"
				// Re-calculate hash for the new content
				hash = sha256.Sum256(content)
				hashStr = hex.EncodeToString(hash[:])
			} else {
				log.Printf("failed to encode jfif as jpg: %v", err)
			}
		} else {
			log.Printf("failed to decode jfif image: %v", err)
		}
	}

	filename := hashStr + ext
	destPath := filepath.Join(destDir, filename)

	err = os.WriteFile(destPath, content, 0644)
	return filename, err
}

func (saver *Saver) Save(idOrUrl string) error {
	if saver.notionToken == "" && saver.notionClient != nil {
		saver.notionToken = string(saver.notionClient.Token)
	}

	thread, err := saver.fetcher.Fetch(context.Background(), idOrUrl)
	if err != nil {
		return err
	}

	if len(thread) == 0 {
		fmt.Println("nothing found to save")
		return nil
	}

	if len(saver.pageTitle) == 0 {
		saver.pageTitle = ExtractTitle(thread[0])
	}

	if saver.outputPath != "" {
		return saver.SaveToDirectory(thread)
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
	return saver.Save(toot)
}
