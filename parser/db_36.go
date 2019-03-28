package parser

import (
	"mgotools/internal"
	"mgotools/mongo"
	"mgotools/parser/logger"
	"mgotools/record"
	"mgotools/util"
)

type Version36Parser struct {
	counters    map[string]string
	versionFlag bool
}

var errorVersion36Unmatched = internal.VersionUnmatched{Message: "version 3.4"}

func init() {
	VersionParserFactory.Register(func() VersionParser {
		return &Version36Parser{
			counters: map[string]string{
				"cursorid":         "cursorid",
				"notoreturn":       "ntoreturn",
				"ntoskip":          "ntoskip",
				"exhaust":          "exhaust",
				"keysExamined":     "keysExamined",
				"docsExamined":     "docsExamined",
				"hasSortStage":     "hasSortStage",
				"fromMultiPlanner": "fromMultiPlanner",
				"replanned":        "replanned",
				"nMatched":         "nmatched",
				"nModified":        "nmodified",
				"ninserted":        "ninserted",
				"ndeleted":         "ndeleted",
				"nreturned":        "nreturned",
				"fastmodinsert":    "fastmodinsert",
				"upsert":           "upsert",
				"cursorExhausted":  "cursorExhausted",
				"nmoved":           "nmoved",
				"keysInserted":     "keysInserted",
				"keysDeleted":      "keysDeleted",
				"writeConflicts":   "writeConflicts",
				"numYields":        "numYields",
				"reslen":           "reslen",
			},

			versionFlag: true,
		}
	})
}

func (v *Version36Parser) Check(base record.Base) bool {
	return base.RawSeverity != record.SeverityNone &&
		base.RawComponent != ""
}

func (v *Version36Parser) NewLogMessage(entry record.Entry) (record.Message, error) {
	r := util.NewRuneReader(entry.RawMessage)
	switch entry.RawComponent {
	case "COMMAND":
		cmd, err := v.command(*r)
		if err != nil {
			return nil, err
		}
		return logger.CrudOrMessage(cmd, cmd.Command, cmd.Counters, cmd.Payload), nil

	case "WRITE":
		op, err := v.operation(*r)
		if err != nil {
			return nil, err
		}
		return logger.CrudOrMessage(op, op.Operation, op.Counters, op.Payload), nil

	case "CONTROL":
		return D(entry).Control(*r)

	case "NETWORK":
		return D(entry).Network(*r)

	case "STORAGE":
		return D(entry).Storage(*r)
	}

	return nil, errorVersion36Unmatched
}

func (v *Version36Parser) command(reader util.RuneReader) (record.MsgCommand, error) {
	r := &reader

	cmd, err := logger.CommandPreamble(r)
	if err != nil {
		return record.MsgCommand{}, err
	}

	if r.ExpectString("originatingCommand") {
		r.SkipWords(1)
		cmd.Payload["originatingCommand"], err = mongo.ParseJsonRunes(r, false)

		if err != nil {
			return record.MsgCommand{}, err
		}
	}

	if r.ExpectString("planSummary:") {
		r.Skip(12).ChompWS()

		cmd.PlanSummary, err = logger.PlanSummary(r)
		if err != nil {
			return record.MsgCommand{}, err
		}
	}

	for {
		param, ok := r.SlurpWord()
		if !ok {
			break
		} else if param == "exception:" {
			cmd.Exception, ok = logger.Exception(r)
			if !ok {
				return record.MsgCommand{}, internal.UnexpectedExceptionFormat
			}
		} else if l := len(param); l > 6 && param[:6] == "locks:" {
			r.RewindSlurpWord()
			break
		} else if !logger.IntegerKeyValue(param, cmd.Counters, v.counters) {
			return record.MsgCommand{}, internal.CounterUnrecognized
		}
	}

	cmd.Locks, err = logger.Locks(r)
	if err != nil {
		return record.MsgCommand{}, err
	}

	cmd.Protocol, err = logger.Protocol(r)
	if err != nil {
		return record.MsgCommand{}, err
	} else if cmd.Protocol != "op_msg" && cmd.Protocol != "op_query" && cmd.Protocol != "op_command" {
		v.versionFlag = false
		return record.MsgCommand{}, errorVersion36Unmatched
	}

	cmd.Duration, err = logger.Duration(r)
	if err != nil {
		return record.MsgCommand{}, err
	}

	return cmd, nil
}

func (v *Version36Parser) operation(reader util.RuneReader) (record.MsgOperation, error) {
	r := &reader

	op, err := logger.OperationPreamble(r)
	if err != nil {
		return record.MsgOperation{}, err
	}

	// Check against the expected list of operations. Anything not in this list
	// is either very broken or a different version.
	if !util.ArrayBinaryMatchString(op.Operation, []string{"command", "commandReply", "compressed", "getmore", "insert", "killcursors", "msg", "none", "query", "remove", "reply", "update"}) {
		v.versionFlag = false
		return record.MsgOperation{}, errorVersion36Unmatched
	}

	// The next word should always be "command:"
	if c, ok := r.SlurpWord(); !ok {
		return record.MsgOperation{}, internal.UnexpectedEOL
	} else if c != "command:" {
		return record.MsgOperation{}, errorVersion36Unmatched
	}

	// There is no bareword like a command (even though the last word was
	// "command:") so the only available option is a JSON document.
	if !r.ExpectRune('{') {
		return record.MsgOperation{}, internal.OperationStructure
	}

	op.Payload, err = mongo.ParseJsonRunes(r, false)
	if err != nil {
		return record.MsgOperation{}, err
	}

	if r.ExpectString("originatingCommand:") {
		r.Skip(19).ChompWS()

		op.Payload["originatingCommand"], err = mongo.ParseJsonRunes(r, false)
		if err != nil {
			return record.MsgOperation{}, err
		}
	}

	if r.ExpectString("planSummary:") {
		r.Skip(12).ChompWS()

		op.PlanSummary, err = logger.PlanSummary(r)
		if err != nil {
			return record.MsgOperation{}, err
		}
	}

	for {
		param, ok := r.SlurpWord()
		if !ok {
			break
		} else if param == "exception:" {
			op.Exception, ok = logger.Exception(r)
			if !ok {
				return record.MsgOperation{}, internal.UnexpectedExceptionFormat
			}
		} else if l := len(param); l > 6 && param[:6] == "locks:" {
			r.RewindSlurpWord()
			break
		} else if !logger.IntegerKeyValue(param, op.Counters, v.counters) {
			return record.MsgOperation{}, internal.CounterUnrecognized
		}
	}

	// Skip "locks:" and resume with JSON.
	r.Skip(6)

	op.Locks, err = mongo.ParseJsonRunes(r, false)
	if err != nil {
		return record.MsgOperation{}, err
	}

	op.Duration, err = logger.Duration(r)
	if err != nil {
		return record.MsgOperation{}, err
	}

	return op, nil
}

func (v *Version36Parser) Version() VersionDefinition {
	return VersionDefinition{Major: 3, Minor: 6, Binary: record.BinaryMongod}
}

func (v *Version36Parser) expectedComponents(c string) bool {
	switch c {
	case "ACCESS",
		"ACCESSCONTROL",
		"ASIO",
		"BRIDGE",
		"COMMAND",
		"CONTROL",
		"DEFAULT",
		"EXECUTOR",
		"FTDC",
		"GEO",
		"HEARTBEATS",
		"INDEX",
		"JOURNAL",
		"NETWORK",
		"QUERY",
		"REPL",
		"REPL_HB",
		"REPLICATION",
		"ROLLBACK",
		"SHARDING",
		"STORAGE",
		"TOTAL",
		"TRACKING",
		"WRITE",
		"-":
		return true
	default:
		return false
	}
}
