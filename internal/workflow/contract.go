// Package workflow defines provider-neutral, offline workflow contracts.
package workflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const MaxEvidence = 1 << 20

type Config struct {
	SchemaVersion int              `json:"schema_version"`
	Exec          []string         `json:"exec"`
	Check         []string         `json:"check"`
	Meter         MeterPolicy      `json:"metering"`
	Candidate     *CandidatePolicy `json:"candidate,omitempty"`
	Score         *ScorePolicy     `json:"score,omitempty"`
}
type MeterPolicy struct {
	Mode      string   `json:"mode"`
	Reserve   []string `json:"reserve,omitempty"`
	Usage     []string `json:"usage,omitempty"`
	MaxCalls  int64    `json:"max_calls,omitempty"`
	MaxTokens int64    `json:"max_tokens,omitempty"`
	Sigma     float64  `json:"sigma,omitempty"`
}
type CandidatePolicy struct {
	Root      string   `json:"root"`
	Include   []string `json:"include"`
	Exclude   []string `json:"exclude"`
	Protected []string `json:"protected"`
	Baseline  bool     `json:"baseline"`
}
type ScorePolicy struct {
	Command        []string `json:"command"`
	Direction      string   `json:"direction"`
	Min            float64  `json:"min"`
	Max            float64  `json:"max"`
	Target         float64  `json:"target"`
	MinImprovement float64  `json:"min_improvement"`
	Tolerance      float64  `json:"tolerance"`
	Patience       int      `json:"patience"`
}
type Request struct {
	SchemaVersion int           `json:"schema_version"`
	RunID         string        `json:"run_id"`
	Iteration     int           `json:"iteration"`
	Phase         string        `json:"phase"`
	ConfigHash    string        `json:"config_hash"`
	CandidateID   string        `json:"candidate_id,omitempty"`
	Reservations  []Reservation `json:"reservations,omitempty"`
}
type Reservation struct {
	ID          string `json:"id"`
	Phase       string `json:"phase"`
	MaxMicroUSD int64  `json:"max_microusd"`
	MaxTokens   int64  `json:"max_tokens"`
}
type Preflight struct {
	SchemaVersion int           `json:"schema_version"`
	RunID         string        `json:"run_id"`
	Iteration     int           `json:"iteration"`
	Phase         string        `json:"phase"`
	Enforced      bool          `json:"enforced"`
	Calls         []Reservation `json:"calls"`
}
type Call struct {
	ID       string `json:"id"`
	Phase    string `json:"phase"`
	Status   string `json:"status"` // actual | unknown | skipped
	MicroUSD *int64 `json:"microusd,omitempty"`
	Tokens   *int64 `json:"tokens,omitempty"`
}
type Usage struct {
	SchemaVersion int    `json:"schema_version"`
	RunID         string `json:"run_id"`
	Iteration     int    `json:"iteration"`
	Phase         string `json:"phase"`
	Calls         []Call `json:"calls"`
}
type Score struct {
	Veto          bool     `json:"veto,omitempty"`
	SchemaVersion int      `json:"schema_version"`
	RunID         string   `json:"run_id"`
	Iteration     int      `json:"iteration"`
	CandidateID   string   `json:"candidate_id"`
	Reviewed      bool     `json:"reviewed"`
	Technical     bool     `json:"technical"`
	Value         *float64 `json:"value,omitempty"`
}

func CallID(run string, iter int, phase, name string) string {
	return fmt.Sprintf("%s/%06d/%s/%s", run, iter, phase, name)
}
func Hash(b []byte) string  { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func JSONHash(v any) string { b, _ := json.Marshal(v); return Hash(b) }
func Decode(b []byte, v any) error {
	if len(b) > MaxEvidence {
		return fmt.Errorf("evidence oversized")
	}
	if e := uniqueKeys(b); e != nil {
		return fmt.Errorf("invalid evidence JSON keys")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return fmt.Errorf("invalid evidence JSON")
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return fmt.Errorf("trailing evidence JSON")
	}
	return nil
}
func Read(path string, v any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("evidence must be a regular file")
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, MaxEvidence+1))
	if e != nil {
		return e
	}
	return Decode(b, v)
}

// Atomic writes to a unique sibling then fsyncs before rename. Caller owns dir.
func Atomic(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".atomic-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	return SyncDirectory(filepath.Dir(path))
}
