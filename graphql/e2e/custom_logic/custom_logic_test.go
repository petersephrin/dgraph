/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package custom_logic

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const (
	alphaURL      = "http://localhost:8180/graphql"
	alphaAdminURL = "http://localhost:8180/admin"
	customTypes   = `type MovieDirector @remote {
		id: ID!
		name: String!
		directed: [Movie]
	}

	type Movie @remote {
		id: ID!
		name: String!
		director: [MovieDirector]
	}
`
)

func updateSchema(t *testing.T, sch string) {
	add := &common.GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					schema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": sch},
	}
	addResult := add.ExecuteAsPost(t, alphaAdminURL)
	require.Nil(t, addResult.Errors)
}

func TestCustomGetQuery(t *testing.T) {
	schema := customTypes + `
	type Query {
        myFavoriteMovies(id: ID!, name: String!, num: Int): [Movie] @custom(http: {
                url: "http://mock:8888/favMovies/$id?name=$name&num=$num",
                method: "GET"
        })
	}`
	updateSchema(t, schema)

	query := `
	query {
		myFavoriteMovies(id: "0x123", name: "Author", num: 10) {
			id
			name
			director {
				id
				name
			}
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)

	expected := `{"myFavoriteMovies":[{"id":"0x3","name":"Star Wars","director":[{"id":"0x4","name":"George Lucas"}]},{"id":"0x5","name":"Star Trek","director":[{"id":"0x6","name":"J.J. Abrams"}]}]}`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomPostQuery(t *testing.T) {
	schema := customTypes + `
	type Query {
        myFavoriteMoviesPost(id: ID!, name: String!, num: Int): [Movie] @custom(http: {
                url: "http://mock:8888/favMoviesPost/$id?name=$name&num=$num",
                method: "POST"
        })
	}`
	updateSchema(t, schema)

	query := `
	query {
		myFavoriteMoviesPost(id: "0x123", name: "Author", num: 10) {
			id
			name
			director {
				id
				name
			}
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)

	expected := `{"myFavoriteMoviesPost":[{"id":"0x3","name":"Star Wars","director":[{"id":"0x4","name":"George Lucas"}]},{"id":"0x5","name":"Star Trek","director":[{"id":"0x6","name":"J.J. Abrams"}]}]}`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomQueryShouldForwardHeaders(t *testing.T) {
	schema := customTypes + `
	type Query {
        verifyHeaders(id: ID!): [Movie] @custom(http: {
                url: "http://mock:8888/verifyHeaders",
				method: "GET",
				forwardHeaders: ["X-App-Token", "X-User-Id"]
        })
	}`
	updateSchema(t, schema)

	query := `
	query {
		verifyHeaders(id: "0x123") {
			id
			name
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
		Headers: map[string][]string{
			"X-App-Token":   []string{"app-token"},
			"X-User-Id":     []string{"123"},
			"Random-header": []string{"random"},
		},
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)
	expected := `{"verifyHeaders":[{"id":"0x3","name":"Star Wars"}]}`
	require.Equal(t, expected, string(result.Data))
}

type teacher struct {
	ID string `json:"tid,omitempty"`
}

func addTeachers(t *testing.T) []*teacher {
	addTeacherParams := &common.GraphQLParams{
		Query: `mutation addTeacher {
			addTeacher(input: [{ age: 28 }, { age: 26 }, { age: 27 }]) {
				teacher {
					tid
				}
			}
		}`,
	}

	result := addTeacherParams.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)

	var res struct {
		AddTeacher struct {
			Teacher []*teacher
		}
	}
	err := json.Unmarshal([]byte(result.Data), &res)
	require.NoError(t, err)

	require.Equal(t, len(res.AddTeacher.Teacher), 3)

	return res.AddTeacher.Teacher
}

type school struct {
	ID string `json:"id,omitempty"`
}

func addSchools(t *testing.T, teachers []*teacher) []*school {

	params := &common.GraphQLParams{
		Query: `mutation addSchool($t1: [TeacherRef], $t2: [TeacherRef], $t3: [TeacherRef]) {
			addSchool(input: [{ established: "1980", teachers: $t1 },
				{ established: "1981", teachers: $t2 }, { established: "1982", teachers: $t3 }]) {
				school {
					id
					teachers {
						tid
					}
				}
			}
		}`,
		Variables: map[string]interface{}{
			// Ofcourse teachers don't work at multiple schools but in our example they do.
			"t1": []map[string]interface{}{{"tid": teachers[0].ID}, {"tid": teachers[1].ID}},
			"t2": []map[string]interface{}{{"tid": teachers[1].ID}, {"tid": teachers[2].ID}},
			"t3": []map[string]interface{}{{"tid": teachers[2].ID}, {"tid": teachers[0].ID}},
		},
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nilf(t, result.Errors, "%+v", result.Errors)

	var res struct {
		AddSchool struct {
			School []*school
		}
	}
	err := json.Unmarshal([]byte(result.Data), &res)
	require.NoError(t, err)

	require.Equal(t, len(res.AddSchool.School), 3)

	return res.AddSchool.School
}

type user struct {
	ID string `json:"id,omitempty"`
}

func addUsers(t *testing.T, schools []*school) []*user {
	params := &common.GraphQLParams{
		Query: `mutation addUser($s1: [SchoolRef], $s2: [SchoolRef], $s3: [SchoolRef]) {
			addUser(input: [{ age: 10, schools: $s1 },
				{ age: 11, schools: $s2 }, { age: 12, schools: $s3 }]) {
				user {
					id
					age
					schools {
						id
						established
					}
				}
			}
		}`,
		Variables: map[string]interface{}{
			// Users could have gone to multiple schools
			"s1": []map[string]interface{}{{"id": schools[0].ID}, {"id": schools[1].ID}},
			"s2": []map[string]interface{}{{"id": schools[1].ID}, {"id": schools[2].ID}},
			"s3": []map[string]interface{}{{"id": schools[2].ID}, {"id": schools[0].ID}},
		},
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nilf(t, result.Errors, "%+v", result.Errors)

	var res struct {
		AddUser struct {
			User []*user
		}
	}
	err := json.Unmarshal([]byte(result.Data), &res)
	require.NoError(t, err)

	require.Equal(t, len(res.AddUser.User), 3)
	for _, u := range res.AddUser.User {
		fmt.Printf("u: %+v\n", u.ID)
	}
	return res.AddUser.User
}

func TestCustomFieldsShouldBeResolved(t *testing.T) {
	d, err := grpc.Dial("localhost:9180", grpc.WithInsecure())
	require.NoError(t, err)

	client := dgo.NewDgraphClient(api.NewDgraphClient(d))
	client.Alter(context.Background(), &api.Operation{DropAll: true})

	schema := `
	type Car @remote {
		id: ID!
		name: String!
	}

	type User {
		id: ID!
		name: String @custom(http: {
						url: "http://mock:8888/userNames",
						method: "GET",
						body: "{uid: $id}"
					})
		age: Int! @search
		cars: [Car] @custom(http: {
						url: "http://mock:8888/cars",
						method: "GET",
						body: "{uid: $id}"
					})
		schools: [School]
	}

	type School {
		id: ID!
		established: String!
		name: String @custom(http: {
						url: "http://mock:8888/schoolNames",
						method: "POST",
						body: "{sid: $id}"
					  })
		classes: [Class] @custom(http: {
							url: "http://mock:8888/classes",
							method: "POST",
							body: "{sid: $id}"
						 })
		teachers: [Teacher]
	}

	type Class @remote {
		id: ID!
		name: String!
		numStudents: Int!
	}

	type Teacher {
		tid: ID!
		age: Int!
		name: String @custom(http: {
						url: "http://mock:8888/teacherNames",
						method: "POST",
						body: "{tid: $tid}"
					  })
	}`

	updateSchema(t, schema)

	teachers := addTeachers(t)
	sort.Slice(teachers, func(i, j int) bool {
		return teachers[i].ID < teachers[i].ID
	})
	schools := addSchools(t, teachers)
	sort.Slice(schools, func(i, j int) bool {
		return schools[i].ID < schools[i].ID
	})
	users := addUsers(t, schools)

	query := `
	query {
		queryUser(order: {asc: age}) {
			name
			age
			cars {
				name
			}
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)
	fmt.Println(string(result.Data))

	expected := `{
		"queryUser": [
		  {
			"name": "uname-` + users[0].ID + `",
			"age": 10,
			"cars": [{
				"name": "BMW"
			}]
		  },
		  {
			"name": "uname-` + users[1].ID + `",
			"age": 11,
			"cars": [{
				"name": "Merc"
			}]
		  },
		  {
			"name": "uname-` + users[2].ID + `",
			"age": 12,
			"cars": [{
				"name": "Honda"
			}]
		  }
		]
	  }`

	testutil.CompareJSON(t, expected, string(result.Data))
}