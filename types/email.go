package types

import "time"

type Email struct {
	MessageId string    `json:"messageId"`
	Date      time.Time `json:"date"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	Content   string    `json:"content"`
}
