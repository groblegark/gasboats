package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
)

// handleGateCommand manages the IP whitelist on Traefik middlewares.
// Usage:
//
//	/gate list                — show current whitelist
//	/gate add <ip>            — add IP (auto-appends /32)
//	/gate remove <ip>         — remove IP from whitelist
func (b *Bot) handleGateCommand(ctx context.Context, cmd slack.SlashCommand) {
	if b.gate == nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":x: Gate (IP whitelist management) is not configured.", false))
		return
	}

	args := strings.Fields(strings.TrimSpace(cmd.Text))
	if len(args) == 0 {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":gate: Usage: `/gate list` · `/gate add <ip>` · `/gate remove <ip>`", false))
		return
	}

	switch args[0] {
	case "list", "ls":
		b.handleGateList(ctx, cmd)
	case "add":
		b.handleGateAdd(ctx, cmd, args[1:])
	case "remove", "rm":
		b.handleGateRemove(ctx, cmd, args[1:])
	default:
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":gate: Usage: `/gate list` · `/gate add <ip>` · `/gate remove <ip>`", false))
	}
}

func (b *Bot) handleGateList(ctx context.Context, cmd slack.SlashCommand) {
	ips, err := b.gate.ListIPs(ctx)
	if err != nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(fmt.Sprintf(":x: Failed to list IPs: %s", err), false))
		return
	}
	if len(ips) == 0 {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":gate: Whitelist is empty (all traffic blocked).", false))
		return
	}
	lines := make([]string, len(ips))
	for i, ip := range ips {
		lines[i] = fmt.Sprintf("• `%s`", ip)
	}
	_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
		slack.MsgOptionText(fmt.Sprintf(":gate: *IP Whitelist* (%d entries)\n%s", len(ips), strings.Join(lines, "\n")), false))
}

func (b *Bot) handleGateAdd(ctx context.Context, cmd slack.SlashCommand, args []string) {
	if len(args) == 0 {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":x: Usage: `/gate add <ip>`", false))
		return
	}
	cidr, err := NormalizeCIDR(args[0])
	if err != nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(fmt.Sprintf(":x: %s", err), false))
		return
	}
	if err := b.gate.AddIP(ctx, cidr); err != nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(fmt.Sprintf(":x: Failed to add %s: %s", cidr, err), false))
		return
	}
	b.logger.Info("gate: IP added via Slack", "cidr", cidr, "user", cmd.UserID)
	_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
		slack.MsgOptionText(fmt.Sprintf(":white_check_mark: Added `%s` to whitelist (all services).", cidr), false))
}

func (b *Bot) handleGateRemove(ctx context.Context, cmd slack.SlashCommand, args []string) {
	if len(args) == 0 {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(":x: Usage: `/gate remove <ip>`", false))
		return
	}
	cidr, err := NormalizeCIDR(args[0])
	if err != nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(fmt.Sprintf(":x: %s", err), false))
		return
	}
	if err := b.gate.RemoveIP(ctx, cidr); err != nil {
		_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
			slack.MsgOptionText(fmt.Sprintf(":x: Failed to remove %s: %s", cidr, err), false))
		return
	}
	b.logger.Info("gate: IP removed via Slack", "cidr", cidr, "user", cmd.UserID)
	_, _ = b.api.PostEphemeral(cmd.ChannelID, cmd.UserID,
		slack.MsgOptionText(fmt.Sprintf(":white_check_mark: Removed `%s` from whitelist (all services).", cidr), false))
}
