package aliases

import (
	"fmt"
	"github.com/ldebruijn/graphql-protect/internal/business/validation"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
	validatorrules "github.com/vektah/gqlparser/v2/validator/rules"
	"testing"
	"time"
)

func Test_MaxAliasesRule(t *testing.T) {
	schema := `
type Query {
   getBook(title: String): Book
}

type Book {
	id: ID!
	title: String
	author: String
}`

	q := `query {
    firstBooks: getBook(title: "null") {
      author
      title
    }
    secondBooks: getBook(title: "null") {
      author
      title
    }
  }`

	type args struct {
		query  string
		schema string
		cfg    Config
	}
	tests := []struct {
		name string
		args args
		want *gqlerror.Error
	}{
		{
			name: "no aliases yields zero count",
			args: args{
				query:  q,
				schema: schema,
				cfg: Config{
					Max:     15,
					Enabled: true,
				},
			},
			want: nil,
		},
		{
			name: "does not produce error when counted aliases are more than configured maximum and reject on failure is false",
			args: args{
				cfg: Config{
					Enabled:         true,
					Max:             1,
					RejectOnFailure: false,
				},
				query:  q,
				schema: schema,
			},
			want: validation.RuleValidationResult{
				Rule:          "max-aliases",
				OperationName: "",
				Result:        validation.REJECTED,
				Message:       fmt.Sprintf("aliases limit of %d exceeded, found %d", 1, 2),
			}.AsGqlError(),
		},
		{
			name: "produces error when counted aliases are more than configured maximum and reject on failure is true",
			args: args{
				cfg: Config{
					Max:             1,
					Enabled:         true,
					RejectOnFailure: true,
				},
				query:  q,
				schema: schema,
			},
			want: validation.RuleValidationResult{
				Rule:          "max-aliases",
				OperationName: "",
				Result:        validation.REJECTED,
				Message:       fmt.Sprintf("aliases limit of %d exceeded, found %d", 1, 2),
			}.AsGqlError(),
		},
		{
			name: "override allows more aliases for named operation",
			args: args{
				query: `query MyOp {
    firstBooks: getBook(title: "null") {
      author
      title
    }
    secondBooks: getBook(title: "null") {
      author
      title
    }
  }`,
				schema: schema,
				cfg: Config{
					Max:             1,
					Enabled:         true,
					RejectOnFailure: true,
					Overrides:       map[string]int{"MyOp": 5},
				},
			},
			want: nil,
		},
		{
			name: "override allows fewer aliases for named operation",
			args: args{
				query: `query MyOp {
    firstBooks: getBook(title: "null") {
      author
      title
    }
    secondBooks: getBook(title: "null") {
      author
      title
    }
  }`,
				schema: schema,
				cfg: Config{
					Max:             15,
					Enabled:         true,
					RejectOnFailure: true,
					Overrides:       map[string]int{"MyOp": 1},
				},
			},
			want: validation.RuleValidationResult{
				Rule:          "max-aliases",
				OperationName: "MyOp",
				Result:        validation.REJECTED,
				Message:       fmt.Sprintf("aliases limit of %d exceeded, found %d", 1, 2),
			}.AsGqlError(),
		},
		{
			name: "override does not apply to other operations",
			args: args{
				query: `query OtherOp {
    firstBooks: getBook(title: "null") {
      author
      title
    }
    secondBooks: getBook(title: "null") {
      author
      title
    }
  }`,
				schema: schema,
				cfg: Config{
					Max:             1,
					Enabled:         true,
					RejectOnFailure: true,
					Overrides:       map[string]int{"MyOp": 5},
				},
			},
			want: validation.RuleValidationResult{
				Rule:          "max-aliases",
				OperationName: "OtherOp",
				Result:        validation.REJECTED,
				Message:       fmt.Sprintf("aliases limit of %d exceeded, found %d", 1, 2),
			}.AsGqlError(),
		},
		{
			name: "respects fragment aliases",
			args: args{
				query: `query A {
		 getBook(title: "null") {
		   firstTitle: title
		   ...BookFragment
		 }
		}
		fragment BookFragment on Book {
		 secondTitle: title
		}`,
				schema: schema,
				cfg: Config{
					Max:             1,
					Enabled:         true,
					RejectOnFailure: true,
				},
			},
			want: validation.RuleValidationResult{
				Rule:          "max-aliases",
				OperationName: "",
				Result:        validation.REJECTED,
				Message:       fmt.Sprintf("aliases limit of %d exceeded, found %d", 1, 2),
			}.AsGqlError(),
		},
		{
			name: "counts aliases hidden inside inline fragments",
			args: args{
				query: `query A {
		 getBook(title: "null") {
		   ... on Book {
		     firstTitle: title
		     secondTitle: title
		   }
		 }
		}`,
				schema: schema,
				cfg: Config{
					Max:             1,
					Enabled:         true,
					RejectOnFailure: true,
				},
			},
			want: validation.RuleValidationResult{
				Rule:          "max-aliases",
				OperationName: "",
				Result:        validation.REJECTED,
				Message:       fmt.Sprintf("aliases limit of %d exceeded, found %d", 1, 2),
			}.AsGqlError(),
		},
		{
			name: "counts aliases in inline fragments nested inside named fragments",
			args: args{
				query: `query A {
		 getBook(title: "null") {
		   ...BookFragment
		 }
		}
		fragment BookFragment on Book {
		 ... on Book {
		   firstTitle: title
		   secondTitle: title
		 }
		}`,
				schema: schema,
				cfg: Config{
					Max:             1,
					Enabled:         true,
					RejectOnFailure: true,
				},
			},
			want: validation.RuleValidationResult{
				Rule:          "max-aliases",
				OperationName: "",
				Result:        validation.REJECTED,
				Message:       fmt.Sprintf("aliases limit of %d exceeded, found %d", 1, 2),
			}.AsGqlError(),
		},
		{
			name: "repeated fragment references are counted per reference",
			args: args{
				query: `query A {
		 getBook(title: "null") {
		   ...BookFragment
		   ...BookFragment
		 }
		}
		fragment BookFragment on Book {
		 firstTitle: title
		}`,
				schema: schema,
				cfg: Config{
					Max:             1,
					Enabled:         true,
					RejectOnFailure: true,
				},
			},
			want: validation.RuleValidationResult{
				Rule:          "max-aliases",
				OperationName: "",
				Result:        validation.REJECTED,
				Message:       fmt.Sprintf("aliases limit of %d exceeded, found %d", 1, 2),
			}.AsGqlError(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := validatorrules.NewDefaultRules()

			NewMaxAliasesRule(tt.args.cfg, rules)

			query, _ := parser.ParseQuery(&ast.Source{Name: "ff", Input: tt.args.query})
			schema := gqlparser.MustLoadSchema(&ast.Source{
				Name:    "graph/schema.graphqls",
				Input:   tt.args.schema,
				BuiltIn: false,
			})

			errs := validator.ValidateWithRules(schema, query, rules)

			if tt.want == nil {
				assert.Empty(t, errs)
			} else {
				assert.Equal(t, tt.want.Message, errs[0].Message)
			}
		})
	}
}

// Test_MaxAliasesRule_MultiOperationAccumulation verifies that the alias limit is enforced
// per-operation and that the counter is not carried over between operations.
func Test_MaxAliasesRule_MultiOperationAccumulation(t *testing.T) {
	schemaStr := `
type Query {
   getBook(title: String): Book
}

type Book {
	id: ID!
	title: String
	author: String
}`

	// Each operation has 6 aliases, limit is 10.
	// A per-operation check should allow both; accumulated check would reject the second.
	query := `
query OpA {
    a1: getBook(title: "1") { title }
    a2: getBook(title: "2") { title }
    a3: getBook(title: "3") { title }
    a4: getBook(title: "4") { title }
    a5: getBook(title: "5") { title }
    a6: getBook(title: "6") { title }
}
query OpB {
    b1: getBook(title: "1") { title }
    b2: getBook(title: "2") { title }
    b3: getBook(title: "3") { title }
    b4: getBook(title: "4") { title }
    b5: getBook(title: "5") { title }
    b6: getBook(title: "6") { title }
}`

	cfg := Config{
		Enabled:         true,
		Max:             10,
		RejectOnFailure: true,
	}

	rules := validatorrules.NewDefaultRules()
	NewMaxAliasesRule(cfg, rules)

	parsedQuery, _ := parser.ParseQuery(&ast.Source{Name: "ff", Input: query})
	schema := gqlparser.MustLoadSchema(&ast.Source{
		Name:    "graph/schema.graphqls",
		Input:   schemaStr,
		BuiltIn: false,
	})

	errs := validator.ValidateWithRules(schema, parsedQuery, rules)

	assert.Empty(t, errs, "each operation has 6 aliases which is below the limit of 10; both should be allowed")
}

// Test_MaxAliasesRule_InlineFragmentPerOperation verifies aliases hidden in
// inline fragments are counted, and that the count stays scoped per operation.
func Test_MaxAliasesRule_InlineFragmentPerOperation(t *testing.T) {
	schemaStr := `
type Query {
   getBook(title: String): Book
}

type Book {
	id: ID!
	title: String
	author: String
}`

	// Each operation hides 2 aliases inside an inline fragment; limit is 3.
	query := `
query OpA {
    getBook(title: "1") {
        ... on Book {
            a1: title
            a2: title
        }
    }
}
query OpB {
    getBook(title: "2") {
        ... on Book {
            b1: title
            b2: title
        }
    }
}`

	cfg := Config{
		Enabled:         true,
		Max:             3,
		RejectOnFailure: true,
	}

	rules := validatorrules.NewDefaultRules()
	NewMaxAliasesRule(cfg, rules)

	parsedQuery, _ := parser.ParseQuery(&ast.Source{Name: "ff", Input: query})
	schema := gqlparser.MustLoadSchema(&ast.Source{
		Name:    "graph/schema.graphqls",
		Input:   schemaStr,
		BuiltIn: false,
	})

	errs := validator.ValidateWithRules(schema, parsedQuery, rules)

	assert.Empty(t, errs, "each operation independently has 2 aliases, below the limit of 3; both should be allowed")
}

// Test_countSelectionSet_CyclicFragments verifies the traversal terminates
// when named fragments reference each other, instead of recursing forever.
func Test_countSelectionSet_CyclicFragments(t *testing.T) {
	fragmentA := &ast.FragmentDefinition{
		Name: "FragmentA",
	}
	fragmentB := &ast.FragmentDefinition{
		Name: "FragmentB",
	}

	fragmentA.SelectionSet = ast.SelectionSet{
		&ast.FragmentSpread{Name: "FragmentB", Definition: fragmentB},
		&ast.Field{Name: "title", Alias: "a1"},
	}
	fragmentB.SelectionSet = ast.SelectionSet{
		&ast.FragmentSpread{Name: "FragmentA", Definition: fragmentA},
		&ast.Field{Name: "title", Alias: "a2"},
	}

	counter := &aliasCounter{
		fragmentAliases: make(map[string]int),
		activeFragments: make(map[string]bool),
	}

	selectionSet := ast.SelectionSet{
		&ast.FragmentSpread{Name: "FragmentA", Definition: fragmentA},
	}

	done := make(chan struct{})
	var count int
	go func() {
		defer close(done)
		count = counter.countSelectionSet(selectionSet)
	}()

	select {
	case <-done:
		assert.Equal(t, 2, count, "both aliases are counted once despite the cyclic reference")
	case <-time.After(2 * time.Second):
		t.Fatal("countSelectionSet did not terminate on cyclic fragment references")
	}
}
