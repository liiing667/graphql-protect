package aliases

import (
	"fmt"
	"github.com/ldebruijn/graphql-protect/internal/business/validation"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
	validatorrules "github.com/vektah/gqlparser/v2/validator/rules"
)

var (
	resultCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "graphql_protect",
		Subsystem: "max_aliases",
		Name:      "results",
		Help:      "The results of the max aliases rule",
	},
		[]string{"result"},
	)
)

type Config struct {
	Enabled         bool           `yaml:"enabled"`
	Max             int            `yaml:"max"`
	RejectOnFailure bool           `yaml:"reject_on_failure"`
	Overrides       map[string]int `yaml:"overrides"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:         true,
		Max:             15,
		RejectOnFailure: true,
		Overrides:       make(map[string]int),
	}
}

func init() {
	prometheus.MustRegister(resultCounter)
}

func NewMaxAliasesRule(cfg Config, rules *validatorrules.Rules) {
	if cfg.Enabled {
		rules.AddRule("MaxAliases", func(observers *validator.Events, addError validator.AddErrFunc) {
			observers.OnOperation(func(_ *validator.Walker, operation *ast.OperationDefinition) {
				// Count everything reachable from this operation in a single pass.
				// The fragment memo is scoped per operation so budgets never leak
				// between operation definitions in the same document.
				counter := &aliasCounter{
					fragmentAliases: make(map[string]int),
					activeFragments: make(map[string]bool),
				}
				aliases := counter.countSelectionSet(operation.SelectionSet)

				maxAliases := cfg.Max
				if override, ok := cfg.Overrides[operation.Name]; ok {
					maxAliases = override
				}

				if aliases > maxAliases {
					if cfg.RejectOnFailure {
						addError(validation.RuleValidationResult{
							Rule:          "max-aliases",
							OperationName: operation.Name,
							Result:        validation.REJECTED,
							Message:       fmt.Sprintf("aliases limit of %d exceeded, found %d", maxAliases, aliases),
						}.Wrap())
						resultCounter.WithLabelValues("rejected").Inc()
					} else {
						addError(validation.RuleValidationResult{
							Rule:          "max-aliases",
							OperationName: operation.Name,
							Result:        validation.FAILED,
							Message:       fmt.Sprintf("aliases limit of %d exceeded, found %d", maxAliases, aliases),
						}.Wrap())
						resultCounter.WithLabelValues("failed").Inc()
					}
				} else {
					resultCounter.WithLabelValues("allowed").Inc()
				}
			})
		})
	}
}

type aliasCounter struct {
	// fragmentAliases memoizes the number of aliases reachable from a named
	// fragment definition, keyed by fragment name.
	fragmentAliases map[string]int
	// activeFragments tracks fragments currently on the traversal stack to
	// guard against cyclic or repeated fragment references.
	activeFragments map[string]bool
}

func (c *aliasCounter) countSelectionSet(set ast.SelectionSet) int {
	count := 0

	for _, selection := range set {
		switch selection := selection.(type) {
		case *ast.Field:
			// When a query has no alias defined it defaults to the name of the query
			if selection.Alias != "" && selection.Alias != selection.Name {
				count++
			}

			count += c.countSelectionSet(selection.SelectionSet)
		case *ast.InlineFragment:
			count += c.countSelectionSet(selection.SelectionSet)
		case *ast.FragmentSpread:
			if selection.Definition == nil {
				continue
			}

			name := selection.Definition.Name

			if memoized, ok := c.fragmentAliases[name]; ok {
				count += memoized
				continue
			}

			// A fragment that is already being expanded references itself,
			// directly or transitively. Stop expanding the cycle; GraphQL
			// validation rejects such documents anyway.
			if c.activeFragments[name] {
				continue
			}

			c.activeFragments[name] = true
			fragmentCount := c.countSelectionSet(selection.Definition.SelectionSet)
			delete(c.activeFragments, name)

			c.fragmentAliases[name] = fragmentCount
			count += fragmentCount
		}
	}

	return count
}
