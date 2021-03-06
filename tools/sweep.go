package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/newrelic/newrelic-client-go/newrelic"
	"github.com/newrelic/newrelic-client-go/pkg/alerts"
	"github.com/newrelic/newrelic-client-go/pkg/entities"
	"github.com/newrelic/newrelic-client-go/pkg/region"
)

func newClient() *newrelic.NewRelic {
	client, err := newrelic.New(
		newrelic.ConfigPersonalAPIKey(os.Getenv("NEW_RELIC_API_KEY")),
		newrelic.ConfigAdminAPIKey(os.Getenv("NEW_RELIC_ADMIN_API_KEY")),
		newrelic.ConfigUserAgent("newrelic/newrelic-client-go (automated account sweeper)"),
		newrelic.ConfigServiceName("integration-test-account-sweeper"),
		newrelic.ConfigRegion(region.Name("US")),
	)

	if err != nil {
		log.Fatalf("failed to initialize client with error: %+v", err)
		return nil
	}

	return client
}

func main() {
	client := newClient()

	if err := deleteApplicationEntities(client); err != nil {
		log.Println(err)
	}

	if err := deletePolicies(client); err != nil {
		log.Println(err)
	}
}

func deleteApplicationEntities(client *newrelic.NewRelic) error {
	applicationEntities, err := client.Entities.SearchEntities(entities.SearchEntitiesParams{
		Domain: entities.EntityDomains.APM,
		Name:   "tf_test",
	})

	if err != nil {
		return fmt.Errorf("ERROR: %+v", err)
	}

	if len(applicationEntities) == 0 {
		log.Println("No entities found.")
		return nil
	}

	ch := make(chan int, len(applicationEntities))
	responses := []int{}
	deleted := []int{}

	for _, e := range applicationEntities {
		if e.ApplicationID != nil {
			// async
			go func(id int) {
				if _, err := client.APM.DeleteApplication(*e.ApplicationID); err != nil {
					log.Printf("[WARN] Error deleting application %v: %v. Continuing to next entity...", *e.ApplicationID, err)
				} else {
					deleted = append(deleted, *e.ApplicationID)
				}

				ch <- *e.ApplicationID
			}(*e.ApplicationID)
		}
	}

	for {
		// do some stuff
		r, ok := <-ch
		if !ok {
			break
		}

		responses = append(responses, r)
		if len(responses) == len(applicationEntities) {
			log.Printf("deleted %d applications", len(deleted))
			close(ch)
		}
	}

	return nil
}

func deletePolicies(client *newrelic.NewRelic) error {
	accountID, _ := strconv.Atoi(os.Getenv("NEW_RELIC_ACCOUNT_ID"))
	policies, err := client.Alerts.QueryPolicySearch(accountID, alerts.AlertsPoliciesSearchCriteriaInput{})

	if err != nil {
		log.Printf("ERROR: %+v", err)
		return err
	}

	log.Printf("Policies count: %v \n", len(policies))

	if len(policies) == 0 {
		log.Println("No policies found.")
		return nil
	}

	ch := make(chan string, len(policies))
	deleted := []string{}

	for _, e := range policies {
		if isIntegrationTestResource(e.Name) {
			// async
			go func(id string) {
				if _, err := client.Alerts.DeletePolicyMutation(accountID, id); err != nil {
					log.Printf("[WARN] Error deleting policy %v: %v. Continuing to next policy...", id, err)
				} else {
					deleted = append(deleted, id)
				}

				ch <- id
			}(e.ID)
		}
	}

	log.Printf("deleted %d policies", len(deleted))

	return nil
}

func isIntegrationTestResource(name string) bool {
	testNames := []string{
		"tf-test",
		"tf_test",
		"k8s-test",
		"test-integration",
	}

	for _, testName := range testNames {
		if strings.Contains(name, testName) {
			return true
		}
	}

	return false
}
