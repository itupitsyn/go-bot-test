package bot

import (
	"context"
	"github.com/go-telegram/bot"
)

func processImgAiQueue(imgChannel chan *aiGenerationProcessorChanel, ctx context.Context, b *bot.Bot) {
	for {
		dataFromChannel := <-imgChannel
		update := dataFromChannel.update
		if update.Message != nil {
			processImageGeneration(ctx, b, update, dataFromChannel.mainMessageId)
		} else if update.CallbackQuery != nil {
			processCallbackQuery(ctx, b, update)
		}
	}
}

func processVideoAiQueue(videoChannel chan *aiGenerationProcessorChanel, ctx context.Context, b *bot.Bot) {
	for {
		dataFromChannel := <-videoChannel
		update := dataFromChannel.update
		mainMessageId := dataFromChannel.mainMessageId
		processVideoGeneration(ctx, b, update, mainMessageId)
	}
}
