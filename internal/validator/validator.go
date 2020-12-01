package validator

import (
	"fantom-api-graphql/internal/config"
	"fantom-api-graphql/internal/logger"
	"fantom-api-graphql/internal/repository"
	"fantom-api-graphql/internal/types"
	"fmt"
	"github.com/ethereum/go-ethereum/common/compiler"
	"strings"
)

// ContractValidator implements the Solidity smart contract
// validator.
type ContractValidator struct {
	repo repository.Repository
	log  logger.Logger
	cfg  *config.Validator
	sig  *config.ServerSignature
}

// NewContractValidator creates a new instance of the contract validator.
func NewContractValidator(cfg *config.Config, repo repository.Repository, log logger.Logger) *ContractValidator {
	// create new instance of the contract validator
	return &ContractValidator{
		repo: repo,
		log:  log,
		cfg:  &cfg.Validator,
		sig:  &cfg.MySignature,
	}
}

// ValidateContract tries to validate contract byte code using
// provided source code. If successful, the contract information
// is updated the the repository and source code hash is pushed
// into the block chain contract registry.
func (cv *ContractValidator) ValidateContract(sc *types.Contract) error {
	// get the byte code of the actual contract
	tx, err := cv.repo.Transaction(&sc.TransactionHash)
	if err != nil {
		cv.log.Errorf("contract deployment not found; %s", err.Error())
		return err
	}

	// is this the expected contract?
	if tx.ContractAddress == nil || !strings.EqualFold(tx.ContractAddress.String(), sc.Address.String()) {
		cv.log.Errorf("invalid contract deployment tx %s for %s", tx.Hash.String(), sc.Address.String())
		return fmt.Errorf("invalid contract details")
	}

	// try to compile the source code provided with the Contract
	artefacts, err := cv.Compile(sc.SourceCode)
	if err != nil {
		cv.log.Errorf("compilation failed; %s", err.Error())
		return err
	}

	// compare artefacts with the deployed contract
	art := cv.MatchArtefact(artefacts, tx.InputData)
	if art == nil {
		return fmt.Errorf("deployed contract does not match the source code provided")
	}

	// mark the contract as validated with the artefact found
	return cv.MarkValidated(sc, art)

	/*
		// loop over contracts ad try to validate one of them
		for name, detail := range contracts {
			// check if the compiled byte code match with the deployed contract
			match, err := compareContractCode(tx, detail.Code)
			if err != nil {
				p.log.Errorf("contract byte code comparison failed")
				return err
			}

			// we have the winner
			if match {
				// set the contract name if not done already
				if 0 == len(sc.Name) {
					sc.Name = strings.TrimPrefix(name, "<stdin>:")
				}

				// update the contract data
				updateContractDetails(sc, detail)

				// write update to the database
				if err := p.db.UpdateContract(sc); err != nil {
					p.log.Errorf("contract validation failed due to db error; %s", err.Error())
					return err
				}

				// inform about success
				p.log.Debugf("contract %s [%s] validated", sc.Address.String(), name)

				// re-scan contract transactions so they are up-to-date with their calls analysis
				p.cache.EvictContract(&sc.Address)
				go p.transactionRescanContractCalls(sc)

				// inform the upper instance we have a winner
				return nil
			}
		}

		// validation fails
		return fmt.Errorf("contract source code does not match with the deployed byte code")
	*/
}
