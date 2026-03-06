package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
)

// handleBouncerCommand manages the IP whitelist on Traefik middlewares.
// Usage:
//
//	/bouncer list                — show current whitelist
//	/bouncer add <ip>            — add IP (auto-appends /32)
//	/bouncer remove <ip>         — remove IP from whitelist
func (b *Bot) handleBouncerCommand(ctx context.Context, cmd slack.SlashCommand) {
	if b.bouncer == nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":x: Bouncer (IP whitelist management) is not configured.", false))
		return
	}

	args := strings.Fields(strings.TrimSpace(cmd.Text))
	if len(args) == 0 {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":door: Usage: `/bouncer list` · `/bouncer add <ip>` · `/bouncer remove <ip>`", false))
		return
	}

	switch args[0] {
	case "list", "ls":
		b.handleBouncerList(ctx, cmd)
	case "add":
		b.handleBouncerAdd(ctx, cmd, args[1:])
	case "remove", "rm":
		b.handleBouncerRemove(ctx, cmd, args[1:])
	default:
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":door: Usage: `/bouncer list` · `/bouncer add <ip>` · `/bouncer remove <ip>`", false))
	}
}

func (b *Bot) handleBouncerList(ctx context.Context, cmd slack.SlashCommand) {
	ips, err := b.bouncer.ListIPs(ctx)
	if err != nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(fmt.Sprintf(":x: Failed to list IPs: %s", err), false))
		return
	}
	if len(ips) == 0 {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":door: Whitelist is empty (all traffic blocked).", false))
		return
	}
	lines := make([]string, len(ips))
	for i, ip := range ips {
		lines[i] = fmt.Sprintf("• `%s`", ip)
	}
	_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
		slack.MsgOptionText(fmt.Sprintf(":door: *IP Whitelist* (%d entries)\n%s", len(ips), strings.Join(lines, "\n")), false))
}

func (b *Bot) handleBouncerAdd(ctx context.Context, cmd slack.SlashCommand, args []string) {
	if len(args) == 0 {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":x: Usage: `/bouncer add <ip>`", false))
		return
	}
	cidr, err := NormalizeCIDR(args[0])
	if err != nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(fmt.Sprintf(":x: %s", err), false))
		return
	}
	if err := b.bouncer.AddIP(ctx, cidr); err != nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(fmt.Sprintf(":x: Failed to add %s: %s", cidr, err), false))
		return
	}
	b.logger.Info("bouncer: IP added via Slack", "cidr", cidr, "user", cmd.UserID)
	_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
		slack.MsgOptionText(fmt.Sprintf(":white_check_mark: Added `%s` to whitelist (all services).", cidr), false))
}

func (b *Bot) handleBouncerRemove(ctx context.Context, cmd slack.SlashCommand, args []string) {
	if len(args) == 0 {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":x: Usage: `/bouncer remove <ip>`", false))
		return
	}
	cidr, err := NormalizeCIDR(args[0])
	if err != nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(fmt.Sprintf(":x: %s", err), false))
		return
	}
	if err := b.bouncer.RemoveIP(ctx, cidr); err != nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(fmt.Sprintf(":x: Failed to remove %s: %s", cidr, err), false))
		return
	}
	b.logger.Info("bouncer: IP removed via Slack", "cidr", cidr, "user", cmd.UserID)
	_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
		slack.MsgOptionText(fmt.Sprintf(":white_check_mark: Removed `%s` from whitelist (all services).", cidr), false))
}
