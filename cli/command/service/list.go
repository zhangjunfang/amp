package service

import (
	"fmt"
	"text/tabwriter"

	"github.com/appcelerator/amp/api/rpc/service"
	"github.com/appcelerator/amp/cli"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type listServiceOptions struct {
	quiet bool
}

// NewServiceListCommand returns a new instance of the service list command.
func NewServiceListCommand(c cli.Interface) *cobra.Command {
	opts := listServiceOptions{}
	cmd := &cobra.Command{
		Use:     "ls [OPTIONS]",
		Short:   "List services",
		Aliases: []string{"list"},
		PreRunE: cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listServices(c, opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false, "Only display team names")
	return cmd
}

func listServices(c cli.Interface, opts listServiceOptions) error {
	conn := c.ClientConn()
	client := service.NewServiceClient(conn)
	request := &service.ServiceListRequest{}
	reply, err := client.ListService(context.Background(), request)
	if err != nil {
		return fmt.Errorf("%s", grpc.ErrorDesc(err))
	}
	if opts.quiet {
		for _, entry := range reply.Entries {
			c.Console().Println(entry.Service.Id)
		}
		return nil
	}
	w := tabwriter.NewWriter(c.Out(), 0, 0, cli.Padding, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tMODE\tREPLICAS\tSTATUS\tIMAGE\tTAG")
	for _, entry := range reply.Entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d/%d\t%s\t%s\t%s\n", entry.Service.Id, entry.Service.Name, entry.Service.Mode, entry.ReadyTasks, entry.TotalTasks, entry.Status, entry.Service.Image, entry.Service.Tag)
	}
	w.Flush()
	return nil
}