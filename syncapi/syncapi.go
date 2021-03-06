// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package syncapi

import (
	"context"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/kafka"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/syncapi/consumers"
	"github.com/matrix-org/dendrite/syncapi/routing"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
)

// AddPublicRoutes sets up and registers HTTP handlers for the SyncAPI
// component.
func AddPublicRoutes(
	router *mux.Router,
	userAPI userapi.UserInternalAPI,
	rsAPI api.RoomserverInternalAPI,
	keyAPI keyapi.KeyInternalAPI,
	federation *gomatrixserverlib.FederationClient,
	cfg *config.SyncAPI,
) {
	consumer, _ := kafka.SetupConsumerProducer(&cfg.Matrix.Kafka)

	syncDB, err := storage.NewSyncServerDatasource(&cfg.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to sync db")
	}

	pos, err := syncDB.SyncPosition(context.Background())
	if err != nil {
		logrus.WithError(err).Panicf("failed to get sync position")
	}

	notifier := sync.NewNotifier(pos)
	err = notifier.Load(context.Background(), syncDB)
	if err != nil {
		logrus.WithError(err).Panicf("failed to start notifier")
	}

	requestPool := sync.NewRequestPool(syncDB, cfg, notifier, userAPI, keyAPI, rsAPI)

	keyChangeConsumer := consumers.NewOutputKeyChangeEventConsumer(
		cfg.Matrix.ServerName, string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputKeyChangeEvent)),
		consumer, notifier, keyAPI, rsAPI, syncDB,
	)
	if err = keyChangeConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start key change consumer")
	}

	roomConsumer := consumers.NewOutputRoomEventConsumer(
		cfg, consumer, notifier, syncDB, rsAPI,
	)
	if err = roomConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start room server consumer")
	}

	clientConsumer := consumers.NewOutputClientDataConsumer(
		cfg, consumer, notifier, syncDB,
	)
	if err = clientConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start client data consumer")
	}

	typingConsumer := consumers.NewOutputTypingEventConsumer(
		cfg, consumer, notifier, syncDB,
	)
	if err = typingConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start typing consumer")
	}

	sendToDeviceConsumer := consumers.NewOutputSendToDeviceEventConsumer(
		cfg, consumer, notifier, syncDB,
	)
	if err = sendToDeviceConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start send-to-device consumer")
	}

	receiptConsumer := consumers.NewOutputReceiptEventConsumer(
		cfg, consumer, notifier, syncDB,
	)
	if err = receiptConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start receipts consumer")
	}

	routing.Setup(router, requestPool, syncDB, userAPI, federation, rsAPI, cfg)
}
