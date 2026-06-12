package raffleLogic

import (
	"fmt"
	"math"
	"telebot/database"
	"telebot/model"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRunRuffle(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectExec("^UPDATE raffle (.+)$").WillReturnResult(sqlmock.NewResult(1, 1))
	const size = 10

	participants := [size]model.User{}

	for i := 0; i < size; i += 1 {
		participants[i] = model.User{
			ID:              int64(i),
			Name:            fmt.Sprintf("User %d", i),
			AlternativeName: fmt.Sprintf("Alt Name %d", i),
		}
	}

	raffle := model.Raffle{
		Participants: participants[:],
		WinnerID:     nil,
	}

	wins := [size]int{}

	for i := 0; i < 10000; i += 1 {
		winner := runRaffle(&raffle)
		wins[winner.ID] += 1
		raffle.WinnerID = nil
	}

	maxDiff := 0

	for i := 0; i < size-1; i += 1 {
		diff := int(math.Abs(float64(wins[i] - wins[i+1])))
		if maxDiff < diff {
			maxDiff = diff
		}
	}

	// Loose statistical bound: with 10000 draws over 10 buckets each bucket
	// averages ~1000, and the max adjacent difference can exceed 100 by chance.
	// 250 keeps the fairness check meaningful while avoiding flakiness.
	allowedDiff := 250

	if maxDiff > allowedDiff {
		t.Errorf("max diff want less than %d got %d", allowedDiff, maxDiff)
	}
}
