package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/events"
	"github.com/spf13/cobra"
)

func eventsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Query the event spine",
	}
	cmd.AddCommand(eventsQueryCmd())
	cmd.AddCommand(eventsSinceCmd())
	return cmd
}

func eventsQueryCmd() *cobra.Command {
	var (
		eventTypes []string
		entityTypes []string
		sourceTools []string
		sinceStr string
		untilStr string
		limit int
		projectPath string
		eventsDB string
	)

	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query events with filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := events.OpenStore(eventsDB)
			if err != nil {
				return err
			}
			defer store.Close()

			filter := events.NewEventFilter().WithLimit(limit)
			for _, t := range eventTypes {
				filter.EventTypes = append(filter.EventTypes, events.EventType(t))
			}
			for _, t := range entityTypes {
				filter.EntityTypes = append(filter.EntityTypes, events.EntityType(t))
			}
			for _, t := range sourceTools {
				filter.SourceTools = append(filter.SourceTools, events.SourceTool(t))
			}
			if sinceStr != "" {
				if since, err := time.Parse(time.RFC3339, sinceStr); err == nil {
					filter.WithSince(since)
				} else {
					return fmt.Errorf("invalid --since: %w", err)
				}
			}
			if untilStr != "" {
				if until, err := time.Parse(time.RFC3339, untilStr); err == nil {
					filter.WithUntil(until)
				} else {
					return fmt.Errorf("invalid --until: %w", err)
				}
			}

			evs, err := store.Query(filter)
			if err != nil {
				return err
			}

			for _, evt := range evs {
				if projectPath != "" && evt.ProjectPath != projectPath {
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s/%s  %s\n",
					evt.CreatedAt.Format(time.RFC3339),
					evt.EventType,
					evt.EntityType,
					evt.EntityID,
					evt.SourceTool,
				)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&eventTypes, "type", nil, "Event type filter (repeatable)")
	cmd.Flags().StringArrayVar(&entityTypes, "entity", nil, "Entity type filter (repeatable)")
	cmd.Flags().StringArrayVar(&sourceTools, "source", nil, "Source tool filter (repeatable)")
	cmd.Flags().StringVar(&sinceStr, "since", "", "Start time (RFC3339)")
	cmd.Flags().StringVar(&untilStr, "until", "", "End time (RFC3339)")
	cmd.Flags().IntVar(&limit, "limit", 200, "Max events to return")
	cmd.Flags().StringVar(&projectPath, "project", "", "Filter by project path")
	cmd.Flags().StringVar(&eventsDB, "events-db", "", "Override events DB path")

	return cmd
}

func eventsSinceCmd() *cobra.Command {
	var (
		projectPath string
		eventsDB string
	)

	cmd := &cobra.Command{
		Use:   "since <timestamp>",
		Short: "List events since a timestamp",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sinceStr := strings.TrimSpace(args[0])
			since, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				return fmt.Errorf("invalid timestamp: %w", err)
			}

			store, err := events.OpenStore(eventsDB)
			if err != nil {
				return err
			}
			defer store.Close()

			filter := events.NewEventFilter().WithSince(since).WithLimit(500)
			evs, err := store.Query(filter)
			if err != nil {
				return err
			}

			for _, evt := range evs {
				if projectPath != "" && evt.ProjectPath != projectPath {
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s/%s  %s\n",
					evt.CreatedAt.Format(time.RFC3339),
					evt.EventType,
					evt.EntityType,
					evt.EntityID,
					evt.SourceTool,
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&projectPath, "project", "", "Filter by project path")
	cmd.Flags().StringVar(&eventsDB, "events-db", "", "Override events DB path")
	cmd.Flags().SetInterspersed(true)

	return cmd
}
