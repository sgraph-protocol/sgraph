package types

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Relation struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	From           string             `bson:"from" json:"from"`
	To             string             `bson:"to" json:"to"`
	Provider       string             `bson:"provider" json:"provider"`
	ConnectedAt    time.Time          `bson:"connected_at" json:"connectedAt"`
	DisconnectedAt *time.Time         `bson:"disconnected_at" json:"disconnectedAt"`
	Extra          []byte             `bson:"extra" json:"extra"`
}
