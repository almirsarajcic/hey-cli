package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/hey-sdk/go/pkg/generated"

	"github.com/basecamp/hey-cli/internal/apierr"
	"github.com/basecamp/hey-cli/internal/mail"
	"github.com/basecamp/hey-cli/internal/output"
)

type contactThreadsCommand struct {
	cmd   *cobra.Command
	limit int
	all   bool
	page  string
}

// contactThreadsOutput is what `hey contact threads --json` answers with: the contact
// by name, HEY's own heading for the list, and the page of threads.
type contactThreadsOutput struct {
	ID           int64                 `json:"id"`
	Name         string                `json:"name,omitempty"`
	EmailAddress string                `json:"email_address,omitempty"`
	EntriesTitle string                `json:"entries_title,omitempty"`
	Postings     []sourcePostingOutput `json:"postings"`
	NextPage     string                `json:"next_page,omitempty"`
}

var contactThreadsListing = postingsListing{
	heading: "Contact",
	summary: func(count int, name string) string {
		return fmt.Sprintf("%d %s with %s", count, threadNoun(count), name)
	},
	cursorNotice: func(shown, total int) string {
		return fmt.Sprintf("Showing %d remaining results from this cursor (%d threads read).", shown, total)
	},
	breadcrumbs: []output.Breadcrumb{
		{Action: "read", Command: "hey thread read <thread-id>", Description: "Read an email thread"},
		{Action: "show", Command: "hey contact show <contact-id>", Description: "View the contact"},
	},
}

func newContactsThreadsCommand() *contactThreadsCommand {
	command := &contactThreadsCommand{}
	command.cmd = &cobra.Command{
		Use:   "threads <id>",
		Short: "List every thread with a contact",
		Long:  "List every email thread a contact is on, seen and unseen — the list HEY heads \"All threads with …\". This is also where a bundle's mail lives once it has been read through.",
		Annotations: map[string]string{
			"agent_notes": "The ID comes from hey contact list, a posting's creator, or hey bundle view's contact. Returns threads newest first with topic_id for hey thread read, and answers --json, --styled, --markdown, --ids-only and --count.",
		},
		Example: `  hey contact threads 12345
  hey contact threads 12345 --all
  hey contact threads 12345 --json`,
		RunE: command.run,
		Args: usageExactOneArg(),
	}

	command.cmd.Flags().IntVar(&command.limit, "limit", 0, "Maximum number of threads to show")
	command.cmd.Flags().BoolVar(&command.all, "all", false, "Fetch all results (override --limit)")
	command.cmd.Flags().StringVar(&command.page, "page", "", "Continue from a next_page cursor")
	return command
}

func (c *contactThreadsCommand) run(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}

	contactID, err := parseContactID(args[0])
	if err != nil {
		return err
	}

	first, err := sdk.Contacts().ThreadsPage(cmd.Context(), contactID, c.page)
	if err != nil {
		return apierr.FromSDK(err)
	}
	if first == nil || first.Contact == nil {
		return apierr.ErrNotFound("contact", args[0])
	}

	contact := first.Contact
	seed := pageResult[generated.Posting]{Items: contact.Postings, Cursor: first.NextPage}
	request := pageRequest{Limit: c.limit, All: c.all, MaxPages: maxPostingPages}

	listing := contactThreadsListing
	listing.payload = func(_ mail.Source, postings []sourcePostingOutput, nextPage string, _ int) any {
		return contactThreadsOutput{
			ID:           contact.Id,
			Name:         contact.Name,
			EmailAddress: contact.EmailAddress,
			EntriesTitle: contact.EntriesTitle,
			Postings:     postings,
			NextPage:     nextPage,
		}
	}
	return listing.write(cmd, mail.ContactSource(contact), seed, request, c.page != "")
}
