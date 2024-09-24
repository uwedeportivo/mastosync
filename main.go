package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	skybot "github.com/danrusei/gobot-bsky"
	"github.com/jomei/notionapi"
	mdon "github.com/mattn/go-mastodon"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mmcdole/gofeed"
	"github.com/neurosnap/sentences"
	td "github.com/neurosnap/sentences/data"
	"github.com/pkg/browser"
	"github.com/urfave/cli"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googdrive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const defaultTmplContent string = `{{.Title}}

{{.Description}}

{{.Link}}
`

const defaultSkyTmplContent string = "{{.Description}}"

func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, path[2:]), nil
}

func configDir(c *cli.Context) (string, error) {
	dir := c.String("config")
	if dir == "" {
		dir = ".mastosync"
		if _, err := os.Stat(dir); err != nil {
			usr, err := user.Current()
			if err != nil {
				return "", err
			}
			dir = filepath.Join(usr.HomeDir, ".mastosync")
		}
	}
	return expandTilde(dir)
}

func handleOauthCallback(codeChan chan string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		queryParts, _ := url.ParseQuery(r.URL.RawQuery)

		// Use the authorization code that is pushed to the redirect URL.
		code := queryParts["code"][0]

		// write the authorization code to the channel
		codeChan <- code

		msg := "<p><strong>Authentication successful</strong>. You may now close this tab.</p>"
		// send a success message to the browser
		if _, err := fmt.Fprint(w, msg); err != nil {
			log.Println(err)
		}
	}
}

func main() {
	app := cli.NewApp()
	app.Name = "mastosync"
	app.Usage = "Toot items from RSS feeds"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config",
			Value: "",
			Usage: "path to a directory",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:    "init",
			Aliases: []string{"i"},
			Usage:   "set up the sync directory",
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				err = os.Mkdir(dir, 0700)
				if err != nil {
					return err
				}
				err = os.Mkdir(filepath.Join(dir, "templates"), 0700)
				if err != nil {
					return err
				}
				err = os.WriteFile(filepath.Join(dir, "templates", "someA.tmpl"), []byte(defaultTmplContent),
					0600)
				err = os.Mkdir(filepath.Join(dir, "skytemplates"), 0700)
				if err != nil {
					return err
				}
				err = os.WriteFile(filepath.Join(dir, "skytemplates", "someA.tmpl"), []byte(defaultSkyTmplContent),
					0600)
				if err != nil {
					return err
				}
				err = InitConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}
				err = CreateDB(filepath.Join(dir, "sync.sqlite3"))
				if err != nil {
					return err
				}
				err = CreateDB(filepath.Join(dir, "skysync.sqlite3"))
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			Name:    "sync",
			Aliases: []string{"s"},
			Usage:   "sync RSS feed with mastodon",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "dryrun",
					Usage: "dryrun the sync",
				},
			},
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				cfg, err := ReadConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}
				dao, err := OpenDB(filepath.Join(dir, "sync.sqlite3"))
				if err != nil {
					return err
				}

				mClient := mdon.NewClient(&cfg.Mas)

				poster := &MastodonPoster{
					mClient: mClient,
				}
				syncer := Syncer{
					feedParser: gofeed.NewParser(),
					poster:     poster,
					dao:        dao,
					feeds:      cfg.Feeds,
					tmplDir:    filepath.Join(dir, "templates"),
					dryrun:     c.Bool("dryrun"),
				}
				return syncer.Sync()
			},
		},
		{
			Name:    "skysync",
			Aliases: []string{"y"},
			Usage:   "sync RSS feed with bluesky",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "dryrun",
					Usage: "dryrun the sync",
				},
			},
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				cfg, err := ReadConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}
				dao, err := OpenDB(filepath.Join(dir, "skysync.sqlite3"))
				if err != nil {
					return err
				}

				var blueAgent *skybot.BskyAgent
				ctx := context.Background()

				agent := skybot.NewAgent(ctx, "https://bsky.social", cfg.BlueSky.Handle, cfg.BlueSky.APIKey)
				err = agent.Connect(ctx)
				if err != nil {
					return err
				}
				blueAgent = &agent
				poster := &BlueskyPoster{
					skyAgent: blueAgent,
				}
				syncer := Syncer{
					feedParser: gofeed.NewParser(),
					poster:     poster,
					dao:        dao,
					feeds:      cfg.SkyFeeds,
					tmplDir:    filepath.Join(dir, "skytemplates"),
					dryrun:     c.Bool("dryrun"),
				}
				return syncer.Sync()
			},
		},
		{
			Name:    "chain",
			Aliases: []string{"x"},
			Usage:   "post a chain of toots",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "dryrun",
					Usage: "dryrun the posting",
				},
				cli.StringFlag{
					Name:  "toots",
					Usage: "path to a txt file containing the toot chain",
				},
			},
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				cfg, err := ReadConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}

				mClient := mdon.NewClient(&cfg.Mas)

				b, err := td.Asset("data/english.json")
				if err != nil {
					return err
				}

				training, err := sentences.LoadTraining(b)
				if err != nil {
					return err
				}

				tokenizer := sentences.NewSentenceTokenizer(training)

				tootsPath := c.String("toots")

				if !filepath.IsAbs(tootsPath) {
					absTootsPath, err := filepath.Abs(tootsPath)
					if err != nil {
						return err
					}
					tootsPath = absTootsPath
				}

				tooter := Tooter{
					mClient:   mClient,
					tootsPath: tootsPath,
					dryrun:    c.Bool("dryrun"),
					tokenizer: tokenizer,
				}
				return tooter.Toot()
			},
		},
		{
			Name:    "catchup",
			Aliases: []string{"c"},
			Usage:   "catchup DB with RSS feed",
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				cfg, err := ReadConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}
				dao, err := OpenDB(filepath.Join(dir, "sync.sqlite3"))
				if err != nil {
					return err
				}

				syncer := Syncer{
					feedParser: gofeed.NewParser(),
					dao:        dao,
					feeds:      cfg.Feeds,
				}
				return syncer.Catchup()
			},
		},
		{
			Name:    "skycatchup",
			Aliases: []string{},
			Usage:   "catchup DB with RSS feed",
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				cfg, err := ReadConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}
				dao, err := OpenDB(filepath.Join(dir, "skysync.sqlite3"))
				if err != nil {
					return err
				}

				syncer := Syncer{
					feedParser: gofeed.NewParser(),
					dao:        dao,
					feeds:      cfg.Feeds,
				}
				return syncer.Catchup()
			},
		},
		{
			Name:    "save",
			Aliases: []string{"n"},
			Usage:   "save a status to notion",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "dryrun",
					Usage: "dryrun the save",
				},
				cli.BoolFlag{
					Name:  "debug",
					Usage: "debug the save",
				},
				cli.BoolFlag{
					Name:  "external",
					Usage: "do not use gdrive to store media, rely on external mastodon server",
				},
				cli.StringFlag{
					Name:  "title",
					Usage: "title of the saved page",
				},
			},
			Action: func(c *cli.Context) error {
				if !c.Args().Present() {
					return fmt.Errorf("missing toot id or url to save")
				}
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				cfg, err := ReadConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}

				notionClient := notionapi.NewClient(notionapi.Token(cfg.NotionToken), notionapi.WithRetry(2))
				mClient := mdon.NewClient(&cfg.Mas)

				b, err := os.ReadFile(filepath.Join(dir, "gdrive.json"))
				if err != nil {
					return err
				}

				// If modifying these scopes, delete your previously saved token.json.
				gdriveConfig, err := google.ConfigFromJSON(b, googdrive.DriveFileScope)
				if err != nil {
					return err
				}

				tokenFile, err := os.Open(filepath.Join(dir, "gdrive.token"))
				if err != nil {
					return err
				}
				gdriveToken := &oauth2.Token{}
				err = json.NewDecoder(tokenFile).Decode(gdriveToken)
				if err != nil {
					return err
				}
				err = tokenFile.Close()
				if err != nil {
					return err
				}
				gdriveClient := gdriveConfig.Client(context.Background(), gdriveToken)

				gdriveService, err := googdrive.NewService(context.Background(),
					option.WithHTTPClient(gdriveClient))
				if err != nil {
					return err
				}

				saver := Saver{
					mClient:        mClient,
					dryrun:         c.Bool("dryrun"),
					notionClient:   notionClient,
					notionParentID: cfg.NotionParent,
					pageTitle:      c.String("title"),
					gdriveService:  gdriveService,
					debug:          c.Bool("debug"),
					usegdrive:      !c.Bool("external"),
					bridge:         cfg.Bridge,
					parent:         cfg.Parent,
				}
				return saver.SaveToot(c.Args().First())
			},
		},
		{
			Name:    "auth",
			Aliases: []string{"a"},
			Usage:   "refresh oauth token for Google Drive",
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				b, err := os.ReadFile(filepath.Join(dir, "gdrive.json"))
				if err != nil {
					return err
				}

				// If modifying these scopes, delete your previously saved token.json.
				gdriveConfig, err := google.ConfigFromJSON(b, googdrive.DriveFileScope)
				if err != nil {
					return err
				}

				tr := &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				}
				sslcli := &http.Client{Transport: tr}
				ctx := context.Background()
				ctx = context.WithValue(ctx, oauth2.HTTPClient, sslcli)

				server := &http.Server{Addr: ":9999"}

				// create a channel to receive the authorization code
				codeChan := make(chan string)

				http.HandleFunc("/oauth/callback", handleOauthCallback(codeChan))

				go func() {
					if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
						log.Fatalf("Failed to start server: %v", err)
					}
				}()

				// get the OAuth authorization URL
				authUrl := gdriveConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

				// Redirect user to consent page to ask for permission
				// for the scopes specified above
				fmt.Printf("Your browser has been opened to visit::\n%s\n", authUrl)

				// open user's browser to login page
				if err := browser.OpenURL(authUrl); err != nil {
					panic(fmt.Errorf("failed to open browser for authentication %v", err))
				}

				authCode := <-codeChan
				tok, err := gdriveConfig.Exchange(context.Background(), authCode)
				if err != nil {
					return err
				}
				f, err := os.OpenFile(filepath.Join(dir, "gdrive.token"),
					os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
				if err != nil {
					return err
				}
				err = json.NewEncoder(f).Encode(tok)
				if err != nil {
					return err
				}
				return f.Close()
			},
		},
		{
			Name:    "mandala",
			Aliases: []string{"m"},
			Usage:   "post a mandala",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "path",
					Value: "/tmp",
					Usage: "path to directory with mandalas",
				},
				cli.StringFlag{
					Name:  "toot",
					Value: "",
					Usage: "toot extra text if specified",
				},
			},
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				cfg, err := ReadConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}

				mClient := mdon.NewClient(&cfg.Mas)

				var blueAgent *skybot.BskyAgent
				ctx := context.Background()

				agent := skybot.NewAgent(ctx, "https://bsky.social", cfg.BlueSky.Handle, cfg.BlueSky.APIKey)
				err = agent.Connect(ctx)
				if err != nil {
					return err
				}
				blueAgent = &agent

				mandala := Mandala{
					mClient:     mClient,
					skyAgent:    blueAgent,
					scriptPath:  cfg.Mandala,
					mandalaPath: c.String("path"),
					tootText:    c.String("toot"),
				}
				return mandala.Post()
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
