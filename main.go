package main

import (
	"fmt"
	"time"

	discord "github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
	gh "github.com/sethvargo/go-githubactions"
)

const CompleteEmoji = "✅"
const WaitingEmoji = "⏳"
const failEmoji = "❌"

// Error
// #CC1111
const RedColor = 0xCC<<16 + 0x11<<8 + 0x11<<0

// #888888 (Skipped/canceled)
const GreyColor = 0x88<<16 + 0x88<<8 + 0x88<<0

// #fc0ff8 (End to End Test)
const PinkColor = 0xFC<<16 + 0x0F<<8 + 0xF8<<0

// #18d6e0 (Compile 1)
const cyanColor = 0x18<<16 + 0xD6<<8 + 0xE0<<0

// #e0cf18 (End to end)
const yellowColor = 0xE0<<16 + 0xCF<<8 + 0x18<<0

// #0088ff (compile 2)
const darkBlueColor = 0x00<<16 + 0x88<<8 + 0xff<<0

// #763e99 (Deploy?)
const PurpleColor = 0x76<<16 + 0x3E<<8 + 0x99<<0

// #66ff00 (Complete)
const GreenColor = 0x66<<16 + 0xff<<8 + 0x00<<0

// #e07018 (Reserved for other errors. Potentially "Tests Skipped" deploy)
const orangeColor = 0xE0<<16 + 0x70<<8 + 0x18<<0

var bot *discord.Session

func main() {

	//check all required inputs
	botToken := gh.GetInput("DISCORD_BOT_TOKEN")
	if botToken == "" {
		gh.Fatalf("DISCORD_BOT_TOKEN is required")
		return
	}
	channel := gh.GetInput("DISCORD_CHANNEL")
	if channel == "" {
		gh.Fatalf("DISCORD_CHANNEL is required")
		return
	}
	stage := gh.GetInput("STAGE")
	if stage == "" {
		gh.Fatalf("STAGE is required")
		return
	}

	var err error
	bot, err = discord.New(botToken)
	if err != nil {
		gh.Fatalf("Failed to create discord bot: %s", err.Error())
		return
	}

	thread := gh.GetInput("DISCORD_THREAD_ID")
	messageID := gh.GetInput("DISCORD_THREAD_MESSAGE_ID")
	stageError := gh.GetInput("STAGE_ERROR")
	canceledMsg := gh.GetInput("CANCELED_MESSAGE")

	if (thread == "") != (messageID == "") {
		gh.Fatalf("Must set both or neither of DISCORD_THREAD_ID and DISCORD_THREAD_MESSAGE_ID")
		return
	}

	if thread == "" || messageID == "" {
		err = startThread()
	} else if stageError != "" {
		err = reportStageError()
	} else if canceledMsg != "" {
		err = reportCanceled()
	} else {
		err = updateThread()
	}

	if err != nil {
		gh.Fatalf("failed to perform stage notice: %s", err.Error())
	}

}

func startThread() (err error) {
	channel := gh.GetInput("DISCORD_CHANNEL")

	color, err := getStageColor()
	if err != nil {
		return errors.Wrap(err, "failed to get color for start thread")
	}

	embedContent, err := getThreadHeaderEmbedContent(false)
	if err != nil {
		return errors.Wrap(err, "failed to get thread header embed content")
	}

	msg, err := bot.ChannelMessageSendComplex(channel, &discord.MessageSend{
		Content: "",
		Embeds: []*discord.MessageEmbed{
			embedContent,
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to send complex message")
	}

	threadTitle, err := getThreadTitle()

	if err != nil {
		return errors.Wrap(err, "failed to get thread title")
	}
	thread, err := bot.MessageThreadStartComplex(channel, msg.ID, &discord.ThreadStart{
		Name:                threadTitle,
		AutoArchiveDuration: 60 * 24 * 7, // archive after 7 days.
	})
	if err != nil {
		return errors.Wrap(err, "failed to start thread from message")
	}

	//send message that the setup is starting
	_, err = bot.ChannelMessageSendComplex(thread.ID, &discord.MessageSend{
		Embeds: []*discord.MessageEmbed{
			{
				Color:       color,
				Description: gh.GetInput("STAGE_STATUS_LONG"),
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to send new message status")
	}

	gh.SetOutput("DISCORD_MESSAGE_ID", msg.ID)
	gh.SetOutput("DISCORD_THREAD_ID", thread.ID)

	return nil
}

func getStageColor() (int, error) {

	stage := gh.GetInput("STAGE")

	color := PinkColor
	switch stage {
	case "test":
		color = PinkColor
	case "build":
		color = cyanColor
	case "e2e":
		color = yellowColor
	case "final-build":
		color = darkBlueColor
	case "deploy":
		color = PurpleColor
	case "complete":
		color = GreenColor
	default:
		return 0, errors.Errorf("Unknown color stage %s", stage)
	}
	return color, nil
}

func updateThread() error {
	channel := gh.GetInput("DISCORD_CHANNEL")
	messageID := gh.GetInput("DISCORD_THREAD_MESSAGE_ID")
	thread := gh.GetInput("DISCORD_THREAD_ID")
	color, err := getStageColor()
	if err != nil {
		return errors.Wrap(err, "failed to get color for update thread")
	}

	embedContent, err := getThreadHeaderEmbedContent(false)
	if err != nil {
		return errors.Wrap(err, "failed to get thread header embed content")
	}

	_, err = bot.ChannelMessageEditComplex(&discord.MessageEdit{
		ID:      messageID,
		Channel: channel,
		Embeds: &[]*discord.MessageEmbed{
			embedContent,
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to edit message message")
	}

	_, err = bot.ChannelMessageSendComplex(thread, &discord.MessageSend{
		Embeds: []*discord.MessageEmbed{
			{
				Color:       color,
				Description: gh.GetInput("STAGE_STATUS_LONG"),
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to send new message status")
	}

	return nil
}
func reportStageError() error {
	channel := gh.GetInput("DISCORD_CHANNEL")
	messageID := gh.GetInput("DISCORD_THREAD_MESSAGE_ID")
	thread := gh.GetInput("DISCORD_THREAD_ID")

	embedContent, err := getThreadHeaderEmbedContent(true)
	if err != nil {
		return errors.Wrap(err, "failed to get thread header embed content")
	}

	TopLevelDesc := gh.GetInput("STAGE_ERROR")
	embedContent.Description = fmt.Sprintf("%s %s %s", failEmoji, TopLevelDesc, failEmoji)

	_, err = bot.ChannelMessageEditComplex(&discord.MessageEdit{
		ID:      messageID,
		Channel: channel,
		Embeds: &[]*discord.MessageEmbed{
			embedContent,
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to edit message message")
	}

	ErrorMessage := gh.GetInput("STAGE_STATUS_LONG")
	_, err = bot.ChannelMessageSendComplex(thread, &discord.MessageSend{
		Content: gh.GetInput("PING_ROLE"),
		Embeds: []*discord.MessageEmbed{
			{
				Color:       RedColor,
				Description: ErrorMessage,
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to send new message status")
	}

	return nil
}

func reportCanceled() error {

	embedContent, err := getThreadHeaderEmbedContent(true)
	if err != nil {
		return errors.Wrap(err, "failed to get thread header embed content")
	}

	embedContent.Description = gh.GetInput("CANCELED_MESSAGE")
	embedContent.Color = GreyColor
	_, err = bot.ChannelMessageEditComplex(&discord.MessageEdit{
		ID:      gh.GetInput("DISCORD_THREAD_MESSAGE_ID"),
		Channel: gh.GetInput("DISCORD_CHANNEL"),
		Embeds: &[]*discord.MessageEmbed{
			embedContent,
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to edit message")
	}

	_, err = bot.ChannelMessageSendComplex(gh.GetInput("DISCORD_THREAD_ID"), &discord.MessageSend{
		Embeds: []*discord.MessageEmbed{
			{
				Color:       GreyColor,
				Description: gh.GetInput("STAGE_STATUS_LONG"),
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to send new message status")
	}

	return nil

}

// helpers
func getRunURL() (string, error) {
	ctx, err := gh.Context()
	if err != nil {
		return "", errors.Wrap(err, "failed to get github context")
	}

	owner, name := ctx.Repo()
	return fmt.Sprintf("%s/%s/%s/actions/runs/%d", ctx.ServerURL, owner, name, ctx.RunID), nil
}

func getEmbedTitle() (string, error) {
	ctx, err := gh.Context()
	if err != nil {
		return "", errors.Wrap(err, "failed to get github context")
	}

	_, service := ctx.Repo()

	environment := ctx.RefName
	return fmt.Sprintf("%s/%s", service, environment), nil
}

func getThreadTitle() (string, error) {
	ctx, err := gh.Context()
	if err != nil {
		return "", errors.Wrap(err, "failed to get github context")
	}
	_, service := ctx.Repo()
	environment := ctx.RefName

	return fmt.Sprintf("%s/%s:%d", service, environment, ctx.RunID), nil
}

func getThreadHeaderEmbedContent(fail bool) (*discord.MessageEmbed, error) {
	runUrl, err := getRunURL()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get run URL")
	}

	fields := []*discord.MessageEmbedField{}
	stage := gh.GetInput("STAGE")
	stageStatus := gh.GetInput("STAGE_STATUS")
	color := GreenColor

	if fail {
		stageStatus = fmt.Sprintf("%s %s %s", failEmoji, stageStatus, failEmoji)
	}

	//set stage status texts.
	switch stage {
	case "test":
		fields = []*discord.MessageEmbedField{
			{Name: "Base Tests", Value: stageStatus, Inline: true},
			{Name: "Compile & Build", Value: WaitingEmoji, Inline: true},
			{Name: "End 2 End", Value: WaitingEmoji, Inline: true},
			{Name: "Re-Tag", Value: WaitingEmoji, Inline: true},
			{Name: "Deploy", Value: WaitingEmoji, Inline: true},
		}
		color = PinkColor
	case "build":
		fields = []*discord.MessageEmbedField{
			{Name: "Base Tests", Value: CompleteEmoji, Inline: true},
			{Name: "Compile & Build", Value: stageStatus, Inline: true},
			{Name: "End 2 End", Value: WaitingEmoji, Inline: true},
			{Name: "Re-Tag", Value: WaitingEmoji, Inline: true},
			{Name: "Deploy", Value: WaitingEmoji, Inline: true},
		}
		color = cyanColor
	case "e2e":
		fields = []*discord.MessageEmbedField{
			{Name: "Base Tests", Value: CompleteEmoji, Inline: true},
			{Name: "Compile & Build", Value: CompleteEmoji, Inline: true},
			{Name: "End 2 End", Value: stageStatus, Inline: true},
			{Name: "Re-Tag", Value: WaitingEmoji, Inline: true},
			{Name: "Deploy", Value: WaitingEmoji, Inline: true},
		}
		color = yellowColor
	case "final-build":
		fields = []*discord.MessageEmbedField{
			{Name: "Base Tests", Value: CompleteEmoji, Inline: true},
			{Name: "Compile & Build", Value: CompleteEmoji, Inline: true},
			{Name: "End 2 End", Value: CompleteEmoji, Inline: true},
			{Name: "Re-Tag", Value: stageStatus, Inline: true},
			{Name: "Deploy", Value: WaitingEmoji, Inline: true},
		}
		color = darkBlueColor
	case "deploy":
		fields = []*discord.MessageEmbedField{
			{Name: "Base Tests", Value: CompleteEmoji, Inline: true},
			{Name: "Compile & Build", Value: CompleteEmoji, Inline: true},
			{Name: "End 2 End", Value: CompleteEmoji, Inline: true},
			{Name: "Re-Tag", Value: CompleteEmoji, Inline: true},
			{Name: "Deploy", Value: stageStatus, Inline: true},
		}
		color = PurpleColor
	case "complete":
		fields = []*discord.MessageEmbedField{
			{Name: "Base Tests", Value: CompleteEmoji, Inline: true},
			{Name: "Compile & Build", Value: CompleteEmoji, Inline: true},
			{Name: "End 2 End", Value: CompleteEmoji, Inline: true},
			{Name: "Re-Tag", Value: CompleteEmoji, Inline: true},
			{Name: "Deploy", Value: CompleteEmoji, Inline: true},
		}
		color = GreenColor
	default:
		return nil, errors.Errorf("Unknown fields stage %s", stage)
	}

	if fail {
		color = RedColor
	}

	embedTitle, err := getEmbedTitle()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embed title for embed content")
	}

	embed := &discord.MessageEmbed{
		Type: discord.EmbedTypeRich,
		Author: &discord.MessageEmbedAuthor{
			URL:  runUrl,
			Name: embedTitle,
		},
		Fields:    fields,
		Color:     color,
		Timestamp: time.Now().Format(time.RFC3339),
		Footer:    &discord.MessageEmbedFooter{},
	}

	return embed, nil
}

//3 main processes.
//Start from scratch
//  No thread or comment ID passed in
//  Create top level comment in destination channel
//  Create thread off of that comment. (Service Environment deployment: Hash)

//Update with current stage info
//  Thread and comment ID must be passed in.
//  Add comment in thread as passed in of the current status
//  Update the top level comment so that it is easily visible
//  Have some special circumstances that stuff can be handled?

//TODO add specific cases for handling/parsing test results

//Report error about stage.
//  Thread and comment ID may be passed in. If missing, will send empty string.
//  If thread/comment exists, Update top level comment with last state, include error message
//  Ping builders as to what SPECIFICALLY failed.
