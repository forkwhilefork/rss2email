//
// This is the push-subcommand.
//

package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	//"strings"

	"github.com/google/subcommands"
    "github.com/gregdel/pushover"
	"github.com/k3a/html2text"
	"github.com/mmcdole/gofeed"
)

// FetchFeed fetches a feed from the remote URL.
//
// We must use this instead of the URL handler that the feed-parser supports
// because reddit, and some other sites, will just return a HTTP error-code
// if we're using a standard "spider" User-Agent.
//
func (p *pushCmd) FetchFeed(url string) (string, error) {
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
func (p *pushCmd) ProcessURL(input string) error {

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

			// If we're supposed to send email then do that
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
				text := html2text.HTML2Text(content)

				// Create a new pushover app with a token
				app := pushover.New(p.apiKey)

				// Create a new recipient
				recipient := pushover.NewRecipient(p.userKey)
			
				// Create the message to send
				message := pushover.NewMessage(text)
			
				// Send the message to the recipient
				_, err := app.SendMessage(message, recipient)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// The options set by our command-line flags.
type pushCmd struct {
	// Should we be verbose in operation?
	verbose bool

	// Emails has the list of emails to which we should send our
	// notices
	emails []string

	// Should we send emails?
	send bool

	// Pushover API key
	apiKey string

	// Pushover user/group key
	userKey string
}

//
// Glue
//
func (*pushCmd) Name() string     { return "push" }
func (*pushCmd) Synopsis() string { return "Process each of the feeds." }
func (*pushCmd) Usage() string {
	return `push :
  Read the list of feeds and send push notification for each new item found in them.
`
}

//
// Flag setup: NOP
//
func (p *pushCmd) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&p.verbose, "verbose", false, "Should we be extra verbose?")
	f.BoolVar(&p.send, "send", true, "Should we send push notifications?")
	f.StringVar(&p.apiKey, "apiKey", "", "Pushover API key")
	f.StringVar(&p.userKey, "userKey", "", "Pushover user (or group) key")
}

//
// Entry-point.
//
func (p *pushCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {

	//
	// Make sure keys are not empty
	//
	if len(p.apiKey) == 0 || len(p.userKey) == 0 {
		fmt.Printf("empty args supplied\nUsage: rss2email push -apiKey=<key> -userKey=<key>\n")
		return subcommands.ExitFailure
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
