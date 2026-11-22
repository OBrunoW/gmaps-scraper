package application

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeLeadsMapsEveryField(t *testing.T) {
	results := json.RawMessage(`[{
		"place_id":" ChIJ1 ","cid":"99","link":" https://maps/1 ","title":" Padaria Central ",
		"phone":" +55 41 3333-0000 ","web_site":"https://padaria.com.br","address":"R. XV, 100",
		"category":"Padaria","complete_address":{"city":"Curitiba","state":"Paraná"},
		"latitude":-25.4284,"longitude":-49.2733
	}]`)

	leads, err := NormalizeLeads(results)

	require.NoError(t, err)
	require.Len(t, leads, 1)

	lead := leads[0]
	require.Equal(t, "place:ChIJ1", lead.Key)
	require.Equal(t, "Padaria Central", lead.Title)
	require.Equal(t, "+55 41 3333-0000", *lead.Phone)
	require.Equal(t, "https://padaria.com.br", *lead.Website)
	require.Equal(t, "R. XV, 100", *lead.Address)
	require.Equal(t, "Curitiba", *lead.City)
	require.Equal(t, "Paraná", *lead.State)
	require.Equal(t, "Padaria", *lead.Category)
	require.Equal(t, "https://maps/1", *lead.MapsURL)
	require.Equal(t, -25.4284, *lead.Lat)
	require.Equal(t, -49.2733, *lead.Lng)
}

func TestNormalizeLeadsKeyPriority(t *testing.T) {
	cases := []struct {
		name    string
		results string
		want    string
	}{
		{"place id first", `[{"title":"A","place_id":"p","cid":"c","link":"l"}]`, "place:p"},
		{"cid second", `[{"title":"A","place_id":"  ","cid":"c","link":"l"}]`, "cid:c"},
		{"link last", `[{"title":"A","link":"l"}]`, "link:l"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			leads, err := NormalizeLeads(json.RawMessage(tc.results))

			require.NoError(t, err)
			require.Len(t, leads, 1)
			require.Equal(t, tc.want, leads[0].Key)
		})
	}
}

func TestNormalizeLeadsDiscardsWithoutTitleOrKey(t *testing.T) {
	results := json.RawMessage(`[
		{"place_id":"p1","title":"   "},
		{"place_id":"p2"},
		{"title":"No key","place_id":" ","cid":"","link":" "},
		{"place_id":"p3","title":"Kept"}
	]`)

	leads, err := NormalizeLeads(results)

	require.NoError(t, err)
	require.Len(t, leads, 1)
	require.Equal(t, "place:p3", leads[0].Key)
}

func TestNormalizeLeadsKeepsFirstDuplicateInOrder(t *testing.T) {
	results := json.RawMessage(`[
		{"place_id":"a","title":"First A"},
		{"place_id":"b","title":"B"},
		{"place_id":"a","title":"Second A"}
	]`)

	leads, err := NormalizeLeads(results)

	require.NoError(t, err)
	require.Len(t, leads, 2)
	require.Equal(t, "First A", leads[0].Title)
	require.Equal(t, "B", leads[1].Title)
}

func TestNormalizeLeadsEmptyTextBecomesNil(t *testing.T) {
	leads, err := NormalizeLeads(json.RawMessage(`[{"place_id":"a","title":"A","phone":"  ","web_site":"","address":"","category":" ","link":"","complete_address":{"city":"","state":""}}]`))

	require.NoError(t, err)
	require.Len(t, leads, 1)

	lead := leads[0]
	require.Nil(t, lead.Phone)
	require.Nil(t, lead.Website)
	require.Nil(t, lead.Address)
	require.Nil(t, lead.Category)
	require.Nil(t, lead.MapsURL)
	require.Nil(t, lead.City)
	require.Nil(t, lead.State)
}

func TestNormalizeLeadsCoordinates(t *testing.T) {
	cases := []struct {
		name    string
		results string
		wantLat *float64
		wantLng *float64
	}{
		{"both zero", `[{"place_id":"a","title":"A","latitude":0,"longitude":0}]`, nil, nil},
		{"absent", `[{"place_id":"a","title":"A"}]`, nil, nil},
		{"longtitude fallback", `[{"place_id":"a","title":"A","latitude":-25.4,"longtitude":-49.2}]`, floatPointer(-25.4), floatPointer(-49.2)},
		{"longitude preferred", `[{"place_id":"a","title":"A","latitude":-25.4,"longitude":-49.2,"longtitude":-1}]`, floatPointer(-25.4), floatPointer(-49.2)},
		{"only latitude", `[{"place_id":"a","title":"A","latitude":-25.4}]`, floatPointer(-25.4), floatPointer(0)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			leads, err := NormalizeLeads(json.RawMessage(tc.results))

			require.NoError(t, err)
			require.Len(t, leads, 1)
			require.Equal(t, tc.wantLat, leads[0].Lat)
			require.Equal(t, tc.wantLng, leads[0].Lng)
		})
	}
}

func TestNormalizeLeadsNonArrayAndEmptyInputs(t *testing.T) {
	for _, input := range []string{``, `null`, `{"a":1}`, `"text"`, `[]`} {
		leads, err := NormalizeLeads(json.RawMessage(input))

		require.NoError(t, err, input)
		require.NotNil(t, leads, input)
		require.Empty(t, leads, input)
	}
}

func TestNormalizeLeadsIgnoresNonObjectItems(t *testing.T) {
	leads, err := NormalizeLeads(json.RawMessage(`[1,"x",null,{"place_id":"a","title":"A"}]`))

	require.NoError(t, err)
	require.Len(t, leads, 1)
}

func TestNormalizeLeadsInvalidJSON(t *testing.T) {
	leads, err := NormalizeLeads(json.RawMessage(`[{"place_id":`))

	require.Error(t, err)
	require.Nil(t, leads)
}

func floatPointer(value float64) *float64 {
	return &value
}
