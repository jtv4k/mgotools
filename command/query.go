package command

// TODO:
//   count by namespace
//   group by IXSCAN
//   group by SORT

import (
	"bytes"
	"math"
	"os"
	"sort"
	"sync"

	"mgotools/internal"
	"mgotools/mongo"
	"mgotools/parser/message"
	"mgotools/parser/version"
	"mgotools/target/formatting"

	"github.com/pkg/errors"
)

const (
	sortNamespace = iota
	sortOperation
	sortPattern
	sortCount
	sortMin
	sortMax
	sortN95
	sortSum
)

const N95MaxSamples = 16 * 1024 * 1024

type query struct {
	Log map[int]*queryInstance

	summaryTable *bytes.Buffer
	wrap         bool
}

type queryInstance struct {
	summary formatting.Summary

	sort []int8

	ErrorCount uint
	LineCount  uint

	Patterns map[string]queryPattern
}

type queryPattern struct {
	formatting.Pattern

	cursorId int64
	p95      []int64
	sync     sync.Mutex
}

var _ Command = (*query)(nil)

func init() {
	args := Definition{
		Usage: "output statistics about query patterns",
		Flags: []Argument{
			{Name: "sort", ShortName: "s", Type: String, Usage: "sort by namespace, pattern, count, min, max, 95%, and/or sum (comma separated for multiple)"},
			{Name: "wrap", Type: Bool, Usage: "line wrapping of query table"},
		},
	}

	init := func() (Command, error) {
		return &query{Log: make(map[int]*queryInstance), summaryTable: bytes.NewBuffer([]byte{}), wrap: false}, nil
	}

	GetFactory().Register("query", args, init)
}

func (s *query) Finish(index int, out commandTarget) error {
	log := s.Log[index]

	values := s.values(log.Patterns)
	s.sort(values, log.sort)

	if index > 0 {
		s.summaryTable.WriteString("\n------------------------------------------\n")
	}

	log.summary.Print(os.Stdout)
	values.Print(s.wrap, s.summaryTable)
	return nil
}

func (s *query) Prepare(name string, instance int, args ArgumentCollection) error {
	s.Log[instance] = &queryInstance{
		Patterns: make(map[string]queryPattern),

		sort:    []int8{sortSum, sortNamespace, sortOperation, sortPattern},
		summary: formatting.NewSummary(name),
	}

	s.wrap = args.Booleans["wrap"]

	sortOptions := map[string]int8{
		"namespace": sortNamespace,
		"operation": sortOperation,
		"pattern":   sortPattern,
		"count":     sortCount,
		"min":       sortMin,
		"max":       sortMax,
		"95%":       sortN95,
		"sum":       sortSum,
	}

	for _, opt := range internal.ArgumentSplit(args.Strings["sort"]) {
		val, ok := sortOptions[opt]
		if !ok {
			return errors.New("unexpected sort option")
		}
		s.Log[instance].sort = append(s.Log[instance].sort, val)
	}

	return nil
}

func (s *query) Run(instance int, out commandTarget, in commandSource, errs commandError) error {
	// Hold a configuration object for future use.
	log := s.Log[instance]

	context := version.New(version.Factory.GetAll(), internal.DefaultDateParser.Clone())
	defer context.Finish()

	// A function to grab new lines and parse them.
	for base := range in {
		log.LineCount += 1

		if base.RawMessage == "" {
			log.ErrorCount += 1
		} else if entry, err := context.NewEntry(base); err != nil {
			log.ErrorCount += 1
		} else {
			// Update the summary with any information available.
			log.summary.Update(entry)

			// Ignore any messages that aren't CRUD related.
			crud, ok := entry.Message.(message.CRUD)
			if !ok {
				// Ignore non-CRUD operations for query purposes.
				continue
			}

			pattern := mongo.NewPattern(crud.Filter)
			query := pattern.StringCompact()

			ns, op, dur, ok := s.standardize(crud)
			if !ok {
				log.ErrorCount += 1
				continue
			}

			op = internal.StringToLower(op)

			switch op {
			case "find":
			case "count":
			case "update":
			case "getmore":
			case "remove":
			case "findandmodify":
			case "geonear":
				// Noop

			default:
				continue
			}

			if op != "" && query != "" {
				key := ns + ":" + op + ":" + query
				pattern, ok := log.Patterns[key]

				if !ok {
					pattern = queryPattern{
						Pattern: formatting.Pattern{
							Min:       math.MaxInt64,
							Namespace: ns,
							Operation: op,
							Pattern:   query,
						},
						p95: make([]int64, 0, N95MaxSamples),
					}
				}

				log.Patterns[key] = s.update(pattern, dur)
			}
		}
	}

	if len(log.summary.Version) == 0 {
		log.summary.Guess(context.Versions())
	}

	return nil
}

func (query) sort(values []formatting.Pattern, order []int8) {
	sort.Slice(values, func(i, j int) bool {
		for _, field := range order {
			switch field {
			case sortNamespace: // Ascending
				if values[i].Namespace == values[j].Namespace {
					continue
				}
				return values[i].Namespace < values[j].Namespace
			case sortOperation: // Ascending
				if values[i].Operation == values[j].Operation {
					continue
				}
				return values[i].Operation < values[j].Operation
			case sortPattern: // Ascending
				if values[i].Pattern == values[j].Pattern {
					continue
				}
				return values[i].Pattern < values[j].Pattern
			case sortSum: // Descending
				if values[i].Sum == values[j].Sum {
					continue
				}
				return values[i].Sum >= values[j].Sum
			case sortN95: // Descending
				if values[i].N95Percentile == values[j].N95Percentile {
					continue
				}
				return values[i].N95Percentile >= values[j].N95Percentile
			case sortMax: // Descending
				if values[i].Max == values[j].Max {
					continue
				}
				return values[i].Max >= values[j].Max
			case sortMin: // Descending
				if values[i].Min == values[j].Min {
					continue
				}
				return values[i].Min >= values[j].Min
			case sortCount: // Descending
				if values[i].Count == values[j].Count {
					continue
				}
				return values[i].Count >= values[j].Count
			}
		}
		return true
	})
}

func (query) standardize(crud message.CRUD) (ns string, op string, dur int64, ok bool) {
	ok = true
	switch cmd := crud.Message.(type) {
	case message.Command:
		dur = cmd.Duration
		ns = cmd.Namespace
		op = cmd.Command

	case message.CommandLegacy:
		dur = cmd.Duration
		ns = cmd.Namespace
		op = cmd.Command

	case message.Operation:
		dur = cmd.Duration
		ns = cmd.Namespace
		op = cmd.Operation

	case message.OperationLegacy:
		dur = cmd.Duration
		ns = cmd.Namespace
		op = cmd.Operation

	default:
		// Returned something completely unexpected so ignore the line.
		ok = false
	}

	return
}

func (s *query) Terminate(out commandTarget) error {
	out <- string(s.summaryTable.String())
	return nil
}

func (query) update(s queryPattern, dur int64) queryPattern {
	s.Count += 1
	s.Sum += dur
	s.p95 = append(s.p95, dur)

	if dur > s.Max {
		s.Max = dur
	}
	if dur < s.Min {
		s.Min = dur
	}

	return s
}

func (s *query) values(patterns map[string]queryPattern) formatting.Table {
	values := make([]formatting.Pattern, 0, len(s.Log))
	for _, pattern := range patterns {
		sort.Slice(pattern.p95, func(i, j int) bool { return pattern.p95[i] <= pattern.p95[j] })

		if len(pattern.p95) > 1 {
			// Get the 95th percent position given the total set of data available.
			index := float64(len(pattern.p95)) * 0.95

			if float64(int64(index)) == index {
				// Check for a whole number (i.e. an exact 95th percentile value).
				pattern.Pattern.N95Percentile = float64(pattern.p95[int(index)])
			} else if index > 1 {
				// Take the average of two values around the 95th percentile.
				pattern.Pattern.N95Percentile = (float64(pattern.p95[int(index)-1] + pattern.p95[int(index)])) / 2
			} else {
				pattern.Pattern.N95Percentile = math.NaN()
			}
		}

		values = append(values, pattern.Pattern)
	}
	return values
}
