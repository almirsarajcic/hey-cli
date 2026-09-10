package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/hey-sdk/go/pkg/generated"

	"github.com/basecamp/hey-cli/internal/apierr"
	"github.com/basecamp/hey-cli/internal/mail"
	"github.com/basecamp/hey-cli/internal/output"
	"github.com/basecamp/hey-cli/internal/terminal"
)

type bundleCommand struct {
	cmd   *cobra.Command
	limit int
	all   bool
	page  string
}

// bundleOutput is what `hey bundle view --json` answers with: the bundled contact next
// to the unseen postings, because the contact's id is what reads the rest of the
// bundle's mail once these threads are seen (hey contact threads <contact-id>).
type bundleOutput struct {
	ID       int64                 `json:"id"`
	Contact  generated.Contact     `json:"contact"`
	Postings []sourcePostingOutput `json:"postings"`
	NextPage string                `json:"next_page,omitempty"`
}

var bundleListing = postingsListing{
	heading: "Bundle",
	summary: func(count int, name string) string {
		return fmt.Sprintf("%d unseen %s bundled from %s", count, threadNoun(count), name)
	},
	cursorNotice: func(shown, total int) string {
		return fmt.Sprintf("Showing %d remaining results from this cursor (%d unseen threads read).", shown, total)
	},
}

func newBundleCommand() *bundleCommand {
	command := newBundleReaderCommand(
		"bundle",
		"List the unseen threads a bundle groups",
		`  hey bundle view 12345
  hey bundle view 12345 --all
  hey bundle view 12345 --json`,
	)
	command.cmd.Annotations[compatibilityUsageAnnotation] = "bundle <box-item-id>"
	command.cmd.Args = cobra.MaximumNArgs(1)
	command.cmd.AddCommand(newBundleViewCommand().cmd)
	return command
}

func newBundleViewCommand() *bundleCommand {
	return newBundleReaderCommand(
		"view <box-item-id>",
		"List the unseen threads a bundle groups",
		`  hey bundle view 12345
  hey bundle view 12345 --page next-cursor
  hey bundle view 12345 --all
  hey bundle view 12345 --json`,
	)
}

func newBundleReaderCommand(use, short, example string) *bundleCommand {
	command := &bundleCommand{}
	command.cmd = &cobra.Command{
		Use:   use,
		Short: short,
		Long:  "List the unseen email threads a bundle groups. A bundle is a hey box view row with kind \"bundle\": one sender's mail rolled into a single row instead of a thread apiece.",
		Annotations: map[string]string{
			"agent_notes": "The ID is a bundle row's own id from hey box view — a row with kind \"bundle\" and no topic_id. Returns the unseen threads the bundle groups, each with topic_id for hey thread read. A bundle read through has no unseen threads; every thread with its sender, seen and unseen, is listed by hey contact threads <contact-id>.",
		},
		Example: example,
		RunE:    command.run,
		Args:    usageExactOneArg(),
	}

	command.cmd.Flags().IntVar(&command.limit, "limit", 0, "Maximum number of threads to show")
	command.cmd.Flags().BoolVar(&command.all, "all", false, "Fetch all results (override --limit)")
	command.cmd.Flags().StringVar(&command.page, "page", "", "Continue from a next_page cursor")
	return command
}

func (c *bundleCommand) run(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	if err := requireAuth(); err != nil {
		return err
	}

	postingID, err := parsePositiveID(args[0], "bundle")
	if err != nil {
		return err
	}

	first, err := sdk.Postings().BundleUnseenPage(cmd.Context(), postingID, c.page)
	if err != nil {
		return bundleNotFound(args[0], apierr.FromSDK(err))
	}
	if first == nil {
		return apierr.ErrNotFound("bundle", args[0])
	}

	contact := first.Contact
	seed := pageResult[generated.Posting]{Items: first.Postings, Cursor: first.NextPage}
	request := pageRequest{Limit: c.limit, All: c.all, MaxPages: maxPostingPages}

	listing := bundleListing
	listing.emptyNotice = fmt.Sprintf(
		"This bundle has no unseen threads — everything in it has been read. List every thread with %s: hey contact threads %d",
		terminal.SanitizeLine(contact.Name), contact.Id)
	listing.breadcrumbs = []output.Breadcrumb{
		{Action: "read", Command: "hey thread read <thread-id>", Description: "Read an email thread"},
		{Action: "contact_threads", Command: fmt.Sprintf("hey contact threads %d", contact.Id),
			Description: "List every thread with this bundle's sender, seen and unseen"},
	}
	listing.payload = func(_ mail.Source, postings []sourcePostingOutput, nextPage string, _ int) any {
		return bundleOutput{ID: postingID, Contact: contact, Postings: postings, NextPage: nextPage}
	}
	return listing.write(cmd, mail.BundleSource(postingID, contact), seed, request, c.page != "")
}

// bundleNotFound says what a 404 on the bundle route means: the ID was not a bundle
// row's. The route answers only for postings that are bundles, so a plain thread's box
// item id and a topic id both 404 here.
func bundleNotFound(identifier string, err error) error {
	var apiErr *apierr.Error
	if errors.As(err, &apiErr) && apiErr.Code == apierr.CodeNotFound {
		return apierr.ErrNotFoundHint("bundle", identifier,
			"The ID must be a bundle row's own id — a hey box view row with kind \"bundle\".")
	}
	return err
}
