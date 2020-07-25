//
// This is the send-subcommand.
//

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
    "net/url"
	"os"
    "strconv"
	"strings"
	"text/template"

	"github.com/google/subcommands"
    "github.com/gregdel/pushover"
	"github.com/mmcdole/gofeed"
	"jaytaylor.com/html2text"
)

// FetchFeed fetches a feed from the remote URL.
//
// We must use this instead of the URL handler that the feed-parser supports
// because reddit, and some other sites, will just return a HTTP error-code
// if we're using a standard "spider" User-Agent.
//
func (p *sendCmd) FetchFeed(url string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "rss2email (https://github.com/skx/rss2email)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	output, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// ProcessURL takes an URL as input, fetches the contents, and then
// processes each feed item found within it.
func (p *sendCmd) ProcessURL(input string) error {

	if p.verbose {
		fmt.Printf("Fetching %s\n", input)
	}

	// Fetch the URL
	txt, err := p.FetchFeed(input)
	if err != nil {
		return fmt.Errorf("error processing %s - %s", input, err.Error())
	}

	// Parse it
	fp := gofeed.NewParser()
	feed, err := fp.ParseString(txt)
	if err != nil {
		return fmt.Errorf("error parsing %s contents: %s", input, err.Error())
	}

	if p.verbose {
		fmt.Printf("\tFound %d entries\n", len(feed.Items))
	}

	// For each entry in the feed ..
	for _, i := range feed.Items {

		// If we've not already notified about this one.
		if !HasSeen(i) {

			if p.verbose {
				fmt.Printf("New item: %s\n", i.GUID)
				fmt.Printf("\tTitle: %s\n", i.Title)
			}

			// Mark the item as having been seen.
			RecordSeen(i)

			// If we're supposed to send any messages
			if p.send {

				// The body should be stored in the
				// "Content" field.
				content := i.Content

				// If the Content field is empty then
				// use the Description instead, if it
				// is non-empty itself.
				if (content == "") && i.Description != "" {
					content = i.Description
				}

				// Convert the content to text.
				text, err := html2text.FromString(content)
				if err != nil {
					panic(err)
				}

				// If we're supposed to send push notifications with pushover
				if p.usePushover {
					// Create a new pushover app with a token
					app := pushover.New(p.pushoverApiKey)

					// Create a new recipient
					recipient := pushover.NewRecipient(p.pushoverUserKey)
				
					// Create the message to send
					message := pushover.NewMessageWithTitle(text, i.Title)

				
					// Send the message to the recipient
					_, err := app.SendMessage(message, recipient)
					if err != nil {
						return err
					}
				}

				if p.useSendy {
					if len(p.emailTemplate) != 0 {
						emailTemplate, err := ioutil.ReadFile(p.emailTemplate)
						if err != nil {
							return err
						}

						type TemplateParms struct {
							Title    string
							Body      string
						}
				
						//
						// Populate it appropriately.
						//
						var x TemplateParms
						x.Title = i.Title
						x.Body = content
				
						//
						// Render our template into a buffer.
						//
						src := string(emailTemplate)
						t := template.Must(template.New("tmpl").Parse(src))
						buf := &bytes.Buffer{}
						err = t.Execute(buf, x)
						if err != nil {
							return err
						}
						content = buf.String()
					}

					apiUrl := "https://" + p.sendyApiHostname
					resource := "/api/campaigns/create.php"
					data := url.Values{}
					data.Set("api_key", p.sendyApiKey)
					data.Set("from_name", p.sendyFromName)
					data.Set("from_email", p.sendyFromEmail)
					data.Set("reply_to", p.sendyFromEmail)
					data.Set("title", i.Title)
					data.Set("html_text", content)
					data.Set("plain_text", text)
					data.Set("list_ids", p.sendyListId)
					data.Set("send_campaign", "1")
					data.Set("subject", i.Title)

					u, _ := url.ParseRequestURI(apiUrl)
					u.Path = resource
					urlStr := u.String()

					client := &http.Client{}
					r, _ := http.NewRequest(http.MethodPost, urlStr, strings.NewReader(data.Encode())) // URL-encoded payload
					r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
					r.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

					resp, err := client.Do(r)
					if err != nil {
						return err
					} else if resp.StatusCode != 200 {
						return errors.New(resp.Status)
					}
				}
			}
		}
	}

	return nil
}

// The options set by our command-line flags.
type sendCmd struct {
	// Should we be verbose in operation?
	verbose bool

	// Emails has the list of emails to which we should send our
	// notices
	emails []string

	// Should we send any messages?
	send bool

	// Should we send push notifications?
	usePushover bool

	// Pushover API key
	pushoverApiKey string

	// Pushover user/group key
	pushoverUserKey string

	// email template file
	emailTemplate string

	// Should we send emails with Sendy?
	useSendy bool

	// Sendy API Hostname
	sendyApiHostname string

	// Sendy API key
	sendyApiKey string
	
	// Sendy destination list ID
	sendyListId string

	// Sendy "from" name
	sendyFromName string
	
	// Sendy "from" email address
	sendyFromEmail string
}

//
// Glue
//
func (*sendCmd) Name() string     { return "send" }
func (*sendCmd) Synopsis() string { return "Process each of the feeds." }
func (*sendCmd) Usage() string {
	return `send :
  Read the list of feeds and send a push notification or an email for each new item found in them.
`
}

//
// Flag setup: NOP
//
func (p *sendCmd) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&p.verbose, "verbose", false, "Should we be extra verbose?")
	f.BoolVar(&p.send, "send", true, "Should we send any messages?")
	f.BoolVar(&p.usePushover, "usePushover", false, "Should we send push messages?")
	f.StringVar(&p.pushoverApiKey, "pushoverApiKey", "", "Pushover API key")
	f.StringVar(&p.pushoverUserKey, "pushoverUserKey", "", "Pushover user (or group) key")
	f.StringVar(&p.emailTemplate, "emailTemplate", "", "Path to email template file")
	f.BoolVar(&p.useSendy, "useSendy", false, "Should we send emails with Sendy?")
	f.StringVar(&p.sendyApiHostname, "sendyApiHostname", "", "Sendy API Hostname (e.g. sendy.example.com)")
	f.StringVar(&p.sendyApiKey, "sendyApiKey", "", "Sendy API key")
	f.StringVar(&p.sendyListId, "sendyListId", "", "Sendy list ID")
	f.StringVar(&p.sendyFromName, "sendyFromName", "", "Sendy from name")
	f.StringVar(&p.sendyFromEmail, "sendyFromEmail", "", "Sendy from email address")
}

//
// Entry-point.
//
func (p *sendCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {

	cmdUsage := "Usage: rss2email send [-send=<true>] [-usePushover=<false> -pushoverApiKey=<key> -pushoverUserKey=<key>]\n" +
				"       [-emailTemplate=</path/to/file>] [-useSendy=<false> -sendyApiHostname=<sendy.example.com>\n" + 
				"        -sendyApiKey=<key> -sendyListId=<key> -sendyFromName=<name> -sendyFromEmail=<email>]"
	
	if (p.usePushover || p.useSendy) {
		//
		// Make sure keys are not empty
		//
		if p.usePushover && (len(p.pushoverApiKey) == 0 || len(p.pushoverUserKey) == 0) {
			fmt.Printf("pushover required parameters missing\n" + cmdUsage + "\n")
			return subcommands.ExitFailure
		}

		//
		// Make sure keys are not empty
		//
		if p.useSendy && (len(p.sendyApiKey) == 0 || len(p.sendyApiHostname) == 0 ||
			len(p.sendyListId) == 0 || len(p.sendyFromName) == 0 || len(p.sendyFromEmail) == 0) {
			
			fmt.Printf("sendy required parameters missing\n" + cmdUsage + "\n")
			return subcommands.ExitFailure
		}
	} else {
		fmt.Printf("you must use either sendy or pushover\n" + cmdUsage + "\n")
		return subcommands.ExitFailure

	}

	if p.useSendy && len(p.emailTemplate) != 0 {
		if _, err := os.Stat(p.emailTemplate); err != nil {
			// p.emailTemplate doesn't exist or can't access
			fmt.Printf("can't stat " + p.emailTemplate + "\n")
			return subcommands.ExitFailure
		}
	}

	//
	// Create the helper
	//
	list := NewFeed()

	//
	// If we receive errors we'll store them here,
	// so we can keep processing subsequent URIs.
	//
	var errors []string

	//
	// For each entry in the list ..
	//
	for _, uri := range list.Entries() {

		//
		// Handle it.
		//
		err := p.ProcessURL(uri)
		if err != nil {
			errors = append(errors, fmt.Sprintf("error processing %s - %s\n", uri, err))
		}
	}

	//
	// If we found errors then handle that.
	//
	if len(errors) > 0 {

		// Show each error to STDERR
		for _, err := range errors {
			fmt.Fprintln(os.Stderr, err)
		}

		// Use a suitable exit-code.
		return subcommands.ExitFailure
	}

	//
	// All good.
	//
	return subcommands.ExitSuccess
}
