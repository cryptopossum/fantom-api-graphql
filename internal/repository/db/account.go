// Package db implements bridge to persistent storage represented by Mongo database.
package db

import (
	"context"
	"fantom-api-graphql/internal/types"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

const (
	// coAccount is the name of the off-chain database collection storing account details.
	coAccounts = "account"

	// fiAccountTransactionPk is the name of the primary key field
	// of the account to transaction collection.
	fiAccountPk = "_id"

	// fiAccountType is the name of the field of the account contract type.
	fiAccountType = "type"

	// fiAccountLastActivity is the name of the field of the account last activity time stamp.
	fiAccountLastActivity = "ats"

	// fiAccountTransactionCounter is the name of the field of the account transaction counter.
	fiAccountTransactionCounter = "atc"

	// fiScCreationTx is the name of the field of the transaction hash
	// which created the contract, if the account is a contract.
	fiScCreationTx = "sc"

	// defaultERC20ListLength is the number of ERC20 tokens pulled by default on negative count
	defaultERC20ListLength = 25
)

// AccountRow is the account base row
type AccountRow struct {
	Address  string       `bson:"_id"`
	Type     string       `bson:"type"`
	Sc       *string      `bson:"sc"`
	Activity uint64       `bson:"ats"`
	Counter  uint64       `bson:"atc"`
	ScHash   *common.Hash `bson:"-"`
}

// initAccountsCollection initializes the account collection with
// indexes and additional parameters needed by the app.
func (db *MongoDbBridge) initAccountsCollection() {
	db.log.Debugf("accounts collection initialized")
}

// Account tries to load an account identified by the address given from
// the off-chain database.
func (db *MongoDbBridge) Account(addr *common.Address) (*types.Account, error) {
	// get the collection for account transactions
	col := db.client.Database(db.dbName).Collection(coAccounts)

	// try to find the account
	sr := col.FindOne(context.Background(), bson.D{{fiAccountPk, addr.String()}}, options.FindOne())

	// error on lookup?
	if sr.Err() != nil {
		// may be ErrNoDocuments, which we seek
		if sr.Err() == mongo.ErrNoDocuments {
			return nil, nil
		}

		db.log.Error("can not get existing account %s; %s", addr.String(), sr.Err().Error())
		return nil, sr.Err()
	}

	// try to decode the row
	var row AccountRow
	err := sr.Decode(&row)
	if err != nil {
		db.log.Error("can not decode account %s; %s", addr.String(), err.Error())
		return nil, err
	}

	// any hash?
	if row.Sc != nil {
		h := common.HexToHash(*row.Sc)
		row.ScHash = &h
	}

	return &types.Account{
		Address:      *addr,
		ContractTx:   row.ScHash,
		Type:         row.Type,
		LastActivity: hexutil.Uint64(row.Activity),
		TrxCounter:   hexutil.Uint64(row.Counter),
	}, nil
}

// AddAccount stores an account in the blockchain if not exists.
func (db *MongoDbBridge) AddAccount(acc *types.Account) error {
	// do we have account data?
	if acc == nil {
		return fmt.Errorf("can not add empty account")
	}

	// get the collection for account transactions
	col := db.client.Database(db.dbName).Collection(coAccounts)

	// extract contract creation transaction if available
	var conTx *string
	if acc.ContractTx != nil {
		cx := acc.ContractTx.String()
		conTx = &cx
	}

	// do the update based on given PK; we don't need to pull the document updated
	_, err := col.InsertOne(context.Background(), bson.D{
		{fiAccountPk, acc.Address.String()},
		{fiScCreationTx, conTx},
		{fiAccountType, acc.Type},
		{fiAccountLastActivity, uint64(acc.LastActivity)},
		{fiAccountTransactionCounter, uint64(acc.TrxCounter)},
	})

	// error on lookup?
	if err != nil {
		db.log.Error("can not insert new account")
		return err
	}

	// check init state
	// make sure transactions collection is initialized
	if db.initAccounts != nil {
		db.initAccounts.Do(func() { db.initAccountsCollection(); db.initAccounts = nil })
	}

	// log what we have done
	db.log.Debugf("added account at %s", acc.Address.String())
	return nil
}

// IsAccountKnown checks if an account document already exists in the database.
func (db *MongoDbBridge) IsAccountKnown(addr *common.Address) (bool, error) {
	// get the collection for account transactions
	col := db.client.Database(db.dbName).Collection(coAccounts)

	// try to find the account in the database (it may already exist)
	sr := col.FindOne(context.Background(), bson.D{
		{fiAccountPk, addr.String()},
	}, options.FindOne().SetProjection(bson.D{{fiAccountPk, true}}))

	// error on lookup?
	if sr.Err() != nil {
		// may be ErrNoDocuments, which we seek
		if sr.Err() == mongo.ErrNoDocuments {
			return false, nil
		}

		db.log.Error("can not get existing account pk")
		return false, sr.Err()
	}

	return true, nil
}

// AccountCount calculates total number of accounts in the database.
func (db *MongoDbBridge) AccountCount() (uint64, error) {
	return db.EstimateCount(db.client.Database(db.dbName).Collection(coAccounts))
}

// AccountTransactions loads list of transaction hashes of an account.
func (db *MongoDbBridge) AccountTransactions(addr *common.Address, cursor *string, count int32) (*types.TransactionList, error) {
	// nothing to load?
	if count == 0 {
		return nil, fmt.Errorf("nothing to do, zero blocks requested")
	}

	// no account given?
	if addr == nil {
		return nil, fmt.Errorf("can not list transactions of empty account")
	}

	// log what we do here
	db.log.Debugf("loading transactions of %s", addr.String())

	// make the filter for [(from = Account) OR (to = Account)]
	filter := bson.D{{"$or", bson.A{bson.D{{"from", addr.String()}}, bson.D{{"to", addr.String()}}}}}

	// return list of transactions filtered by the account
	return db.Transactions(cursor, count, &filter)
}

// AccountMarkActivity marks the latest account activity in the repository.
func (db *MongoDbBridge) AccountMarkActivity(addr *common.Address, ts uint64) error {
	// log what we do
	db.log.Debugf("account %s activity at %s", addr.String(), time.Unix(int64(ts), 0).String())

	// get the collection for contracts
	col := db.client.Database(db.dbName).Collection(coAccounts)

	// update the contract details
	if _, err := col.UpdateOne(context.Background(),
		bson.D{{fiAccountPk, addr.String()}},
		bson.D{
			{"$set", bson.D{{fiAccountLastActivity, ts}}},
			{"$inc", bson.D{{fiAccountTransactionCounter, 1}}},
		}); err != nil {
		// log the issue
		db.log.Errorf("can not update account %s details; %s", addr.String(), err.Error())
		return err
	}

	return nil
}

// Erc20TokensList returns a list of known ERC20 tokens ordered by their activity.
func (db *MongoDbBridge) Erc20TokensList(count int32) ([]common.Address, error) {
	// make sure the count is positive; use default size if not
	if count <= 0 {
		count = defaultERC20ListLength
	}

	// log what we do
	db.log.Debugf("loading %d most active ERC20 token accounts", count)

	// get the collection for contracts
	col := db.client.Database(db.dbName).Collection(coAccounts)

	// make the filter for ERC20 tokens only and pull them ordered by activity
	filter := bson.D{{"type", types.AccountTypeERC20Token}}
	opt := options.Find().SetSort(bson.D{
		{fiAccountTransactionCounter, -1},
		{fiAccountLastActivity, -1},
	}).SetLimit(int64(count))

	// load the data
	cursor, err := col.Find(context.Background(), filter, opt)
	if err != nil {
		db.log.Errorf("error loading ERC20 tokens list; %s", err.Error())
		return nil, err
	}

	return db.loadErc20TokensList(cursor)
}

// Erc20TokensList returns a list of known ERC20 tokens ordered by their activity.
func (db *MongoDbBridge) loadErc20TokensList(cursor *mongo.Cursor) ([]common.Address, error) {
	// close the cursor as we leave
	defer func() {
		err := cursor.Close(context.Background())
		if err != nil {
			db.log.Errorf("error closing ERC20 list cursor; %s", err.Error())
		}
	}()

	// loop and load
	list := make([]common.Address, 0)
	var row AccountRow
	for cursor.Next(context.Background()) {
		// try to decode the next row
		if err := cursor.Decode(&row); err != nil {
			db.log.Errorf("can not decodeERC20 list row; %s", err.Error())
			return nil, err
		}

		// decode the value
		list = append(list, common.HexToAddress(row.Address))
	}

	return list, nil
}
