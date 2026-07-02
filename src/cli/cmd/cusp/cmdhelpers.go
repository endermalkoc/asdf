package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/store"
)

// Small command-shape helpers shared by the higher-volume verb groups (the testing layer), so
// each command stays a few lines. They wrap the same connect/connectRead/runMutate/emit
// primitives every other command uses.

// simpleList runs a read and emits the result (JSON, or the human string the reader builds).
func simpleList(cmd *cobra.Command, fn func(ctx context.Context, r store.Execer) (any, string, error)) error {
	ctx := cmd.Context()
	r, done, err := connectRead(ctx)
	if err != nil {
		return err
	}
	defer done()
	v, human, err := fn(ctx, r)
	if err != nil {
		return err
	}
	if flagJSON {
		emit(v, "")
		return nil
	}
	fmt.Print(human)
	return nil
}

// simpleGet reads one row by key via a store getter; NotFound when absent, else emits it (JSON,
// or a reflected field dump in human mode).
func simpleGet[T any](cmd *cobra.Command, get func(context.Context, store.Execer, string) (T, bool, error), kind, key string) error {
	ctx := cmd.Context()
	r, done, err := connectRead(ctx)
	if err != nil {
		return err
	}
	defer done()
	v, ok, err := get(ctx, r, key)
	if err != nil {
		return err
	}
	if !ok {
		return app.NotFound(kind, key)
	}
	if flagJSON {
		emit(v, "")
		return nil
	}
	fmt.Print(printStruct(v))
	return nil
}

// simpleMutate routes a write through runMutate; body returns the value to emit + its human line.
func simpleMutate(cmd *cobra.Command, summary string,
	validate func(ctx context.Context, r store.Execer) error,
	body func(ctx context.Context, w *app.Write) (any, string, error)) error {
	ctx := cmd.Context()
	ws, err := connect(ctx)
	if err != nil {
		return err
	}
	defer ws.Close()
	var outV any
	var human string
	err = runMutate(cmd, ws, app.MutateOpts{Summary: summary, Validate: validate}, func(ctx context.Context, w *app.Write) error {
		v, h, e := body(ctx, w)
		if e != nil {
			return e
		}
		outV, human = v, h
		return nil
	})
	if err != nil {
		return err
	}
	emit(outV, human)
	return nil
}

// simpleDelete deletes one row by id via a store deleter; NotFound when absent, marks dirty tables.
func simpleDelete(cmd *cobra.Command, summary, kind, key string,
	del func(context.Context, store.Execer, string) (bool, error), dirty ...string) error {
	return simpleMutate(cmd, summary, nil, func(ctx context.Context, w *app.Write) (any, string, error) {
		ok, e := del(ctx, w.Tx, key)
		if e != nil {
			return nil, "", e
		}
		if !ok {
			return nil, "", app.NotFound(kind, key)
		}
		for _, t := range dirty {
			w.MarkDirty(t)
		}
		return map[string]any{"id": key}, fmt.Sprintf("deleted %s %s", kind, key), nil
	})
}

// applyStr copies val into dst only when the named flag was passed (partial edits).
func applyStr(cmd *cobra.Command, flag, val string, dst *string) {
	if cmd.Flags().Changed(flag) {
		*dst = val
	}
}

// printStruct renders a row's exported, non-empty fields as "name: value" lines for human show
// output, using the json tag for the field name (skipping nil pointers and empty strings).
func printStruct(v any) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	rt := rv.Type()
	var b strings.Builder
	for i := 0; i < rt.NumField(); i++ {
		name := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			name = rt.Field(i).Name
		}
		fv := rv.Field(i)
		if fv.Kind() == reflect.Ptr {
			if fv.IsNil() {
				continue
			}
			fv = fv.Elem()
		}
		s := fmt.Sprintf("%v", fv.Interface())
		if s == "" || s == "false" {
			continue
		}
		fmt.Fprintf(&b, "  %-16s %s\n", name+":", s)
	}
	return b.String()
}
