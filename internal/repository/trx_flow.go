/*
Package repository implements repository for handling fast and efficient access to data required
by the resolvers of the API server.

Internally it utilizes RPC to access Opera/Lachesis full node for blockchain interaction. Mongo database
for fast, robust and scalable off-chain data storage, especially for aggregated and pre-calculated data mining
results. BigCache for in-memory object storage to speed up loading of frequently accessed entities.
*/
package repository

import (
	"fantom-api-graphql/internal/logger"
	"fantom-api-graphql/internal/types"
	"sync"
	"time"
)

const (
	// trxFlowUpdaterPeriod represents the period in which we do trx flow updates.
	trxFlowUpdaterPeriod = 7 * time.Minute

	// trxCountUpdaterPeriod represents the period in which the trx count estimation
	// is updated from the underlying database.
	trxCountUpdaterPeriod = 30 * time.Minute

	// trxFlowUpdateRange represents the range for which we do the trx flow update.
	trxFlowUpdateRange = -2 * 24 * time.Hour
)

// txFlowUpdater represents a service for regular updates of the TX Flow database records.
type txFlowUpdater struct {
	service
}

// NewTxFlowUpdater creates a new TX Flow updater service.
func NewTxFlowUpdater(repo Repository, log logger.Logger, wg *sync.WaitGroup) *txFlowUpdater {
	return &txFlowUpdater{
		service: newService("trx flow updater", repo, log, wg),
	}
}

// run starts the tx flow updater service
func (tfu *txFlowUpdater) run() {
	// start go routine for processing
	tfu.wg.Add(1)
	go tfu.schedule()
}

// schedule schedules regular TX flow updates.
func (tfu *txFlowUpdater) schedule() {
	// inform about the monitor
	tfu.log.Notice("trx flow updater is running")

	// make tickers
	flowTicker := time.NewTicker(trxFlowUpdaterPeriod)
	trxCountTicker := time.NewTicker(trxCountUpdaterPeriod)

	// don't forget to sign off after we are done
	defer func() {
		// stop the ticker
		flowTicker.Stop()
		trxCountTicker.Stop()

		// log finish and signal end
		tfu.log.Notice("trx flow updater is closed")
		tfu.wg.Done()
	}()

	// do initial update
	go tfu.updateTrxCountEstimate()

	// loop here
	for {
		select {
		case <-tfu.sigStop:
			return
		case <-flowTicker.C:
			tfu.log.Infof("calling for trx flow update")
			tfu.repo.TrxFlowUpdate()
		case <-trxCountTicker.C:
			tfu.log.Infof("calling for trx count update")
			go tfu.updateTrxCountEstimate()
		}
	}
}

// updateTrxCountEstimate updates trx counter estimation.
func (tfu *txFlowUpdater) updateTrxCountEstimate() {
	// pull the value from DB
	val, err := tfu.repo.TransactionsCount()
	if err != nil {
		tfu.log.Errorf("can not update trx count estimation; %s", err.Error())
		return
	}

	// update the estimate
	tfu.repo.UpdateTrxCountEstimate(val)
}

// TrxFlowVolume resolves the list of daily trx flow aggregations.
func (p *proxy) TrxFlowVolume(from *time.Time, to *time.Time) ([]*types.DailyTrxVolume, error) {
	return p.db.TrxDailyFlowList(from, to)
}

// TrxFlowSpeed provides speed of transaction per second for the last <sec> seconds.
func (p *proxy) TrxFlowSpeed(sec int32) (float64, error) {
	return p.db.TrxRecentTrxSpeed(sec)
}

// TrxGasSpeed provides speed of gas consumption per second by transactions.
func (p *proxy) TrxGasSpeed(from *time.Time, to *time.Time) (float64, error) {
	return p.db.TrxGasSpeed(from, to)
}

// TrxFlowUpdate executes the trx flow update in the database.
func (p *proxy) TrxFlowUpdate() {
	// calculate previous midnight
	now := time.Now().UTC()
	h, m, s := now.Clock()
	from := now.Add(time.Duration(-(h*3600 + m*60 + s)) * time.Second).Add(time.Duration(-now.Nanosecond()) * time.Nanosecond).Add(trxFlowUpdateRange)

	// do the update
	err := p.db.TrxDailyFlowUpdate(from)
	if err != nil {
		p.log.Criticalf("can not update trx flow; %s", err.Error())
	}

	// log success
	p.log.Debugf("trx flow updated")
}
