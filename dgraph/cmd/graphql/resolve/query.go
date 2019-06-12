/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package resolve

import (
	"errors"
	"strconv"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/vektah/gqlparser/gqlerror"
)

type queryBuilder struct {
	graphQuery *gql.GraphQuery
	err        error
}

func (r *RequestResolver) resolveQuery(q schema.Query) {
	// all queries in an operation should run regardless of errors in other queries

	var gq *gql.GraphQuery

	// currently only handles getT(id: "0x123") queries
	// FIXME: "q.QueryType()"
	switch schema.GetQuery {
	case schema.GetQuery:
		qb := newQueryBuilder()
		qb.withAttr(q.ResponseName())
		qb.withIDArgRoot(q)
		qb.withTypeFilter(q)
		qb.withSelectionSetFrom(q)
		// TODO: also builder.withPagination() ... etc ...

		var err error
		gq, err = qb.query()
		if err != nil {
			// FIXME: could be a bug like error here or a gql error

			// TODO: errors that probably mean bugs, should return a simple GraphQL error
			// AND log with a guid linking the GraphQL error and the server log
			// errID := uuid.New().String() ... etc
		}

	default:
		r.WithErrors(gqlerror.Errorf("Only get queries are implemented"))
		return
	}

	res, err := executeQuery(gq, r.dgraphClient)
	if err != nil {
		r.WithErrors(gqlerror.Errorf("Failed to query dgraph with error : %s", err))
		glog.Infof("Dgraph query failed : %s", err) // maybe log more info if it could be a bug?
	}

	// TODO:
	// More is needed here if we are to be totally GraphQL compliant.
	// e.g. need to dig through that response and the expected types from the schema
	// and propagate errors and missing ! fields according to spec.
	r.resp.Data.Write(res)
}

func newQueryBuilder() *queryBuilder {
	return &queryBuilder{
		graphQuery: &gql.GraphQuery{},
	}
}

func (qb *queryBuilder) withAttr(attr string) {
	if qb == nil || qb.graphQuery == nil || qb.err != nil {
		return
	}

	qb.graphQuery.Attr = attr
}

func (qb *queryBuilder) withAlias(alias string) {
	if qb == nil || qb.graphQuery == nil || qb.err != nil {
		return
	}

	qb.graphQuery.Alias = alias
}

func (qb *queryBuilder) withIDArgRoot(q schema.Query) {
	if qb == nil || qb.graphQuery == nil || qb.err != nil {
		return
	}

	idArg, err := q.ArgValue(idArgName)
	if err != nil || idArg == nil {
		qb.err = errors.New("ID arg not found (should be impossible in a valid schema)")
	}

	id, ok := idArg.(string)
	if !ok {
		qb.err = errors.New("ID arg not a string (should be impossible in a valid schema)")
	}

	uid, err := strconv.ParseUint(id, 0, 64)
	if err != nil {
		// FIXME: actually this should surface as a GraphQL error
		qb.err = errors.New("ID argument wasn't a valid ID")
	}

	qb.withUIDRoot(uid)
}

func (qb *queryBuilder) withUIDRoot(uid uint64) {
	if qb == nil || qb.graphQuery == nil || qb.err != nil {
		return
	}

	qb.graphQuery.Func = &gql.Function{
		Name: "uid",
		UID:  []uint64{uid},
	}
}

func (qb *queryBuilder) withTypeFilter(f schema.Field) {
	if qb == nil || qb.graphQuery == nil || qb.err != nil {
		return
	}

	qb.graphQuery.Filter = &gql.FilterTree{
		Func: &gql.Function{
			Name: "type",
			Args: []gql.Arg{{Value: f.TypeName()}},
		},
	}
}

func (qb *queryBuilder) withSelectionSetFrom(fld schema.Field) {
	if qb == nil || qb.graphQuery == nil || qb.err != nil {
		return
	}

	for _, f := range fld.SelectionSet() {
		qbld := newQueryBuilder()
		if f.Alias() != "" {
			qbld.withAlias(f.Alias())
		} else {
			qbld.withAlias(f.Name())
		}
		qbld.withAttr(fld.TypeName() + "." + f.Name())
		// TODO: filters, pagination, etc in here
		qbld.withSelectionSetFrom(f)

		q, err := qbld.query()
		if err != nil {
			qb.err = err
			return
		}

		qb.graphQuery.Children = append(qb.graphQuery.Children, q)
	}
}

func (qb *queryBuilder) query() (*gql.GraphQuery, error) {
	if qb == nil {
		return nil, errors.New("nil query builder")
	}

	return qb.graphQuery, qb.err
}