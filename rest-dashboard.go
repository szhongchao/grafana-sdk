package sdk

/*
   Copyright 2016 Alexander I.Grafov <grafov@gmail.com>

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.

   ॐ तारे तुत्तारे तुरे स्व
*/

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// BoardProperties keeps metadata of a dashboard.
type BoardProperties struct {
	IsStarred  bool      `json:"isStarred,omitempty"`
	IsHome     bool      `json:"isHome,omitempty"`
	IsSnapshot bool      `json:"isSnapshot,omitempty"`
	Type       string    `json:"type,omitempty"`
	CanSave    bool      `json:"canSave"`
	CanEdit    bool      `json:"canEdit"`
	CanStar    bool      `json:"canStar"`
	Slug       string    `json:"slug"`
	Expires    time.Time `json:"expires"`
	Created    time.Time `json:"created"`
	Updated    time.Time `json:"updated"`
	UpdatedBy  string    `json:"updatedBy"`
	CreatedBy  string    `json:"createdBy"`
	Version    int       `json:"version"`
}

// GetDashboardByUID loads a dashboard and its metadata from Grafana by dashboard uid.
func (r *Client) GetDashboardByUID(uid string) (Board, BoardProperties, error) {
	return r.getDashboard("uid/" + uid)
}

// GetDashboardBySlug loads a dashboard and its metadata from Grafana by dashboard slug.
// Deprecated since Grafana v5
//
// For dashboards from a filesystem set "file/" prefix for slug. By default dashboards from
// a database assumed. Database dashboards may have "db/" prefix or may have not, it will
// be appended automatically.
func (r *Client) GetDashboardBySlug(slug string) (Board, BoardProperties, error) {
	path, _ := setPrefix(slug)
	return r.getDashboard(path)
}

func (r *Client) getDashboard(path string) (Board, BoardProperties, error) {
	var (
		raw    []byte
		result struct {
			Meta  BoardProperties `json:"meta"`
			Board Board           `json:"dashboard"`
		}
		code int
		err  error
	)
	if raw, code, err = r.get(fmt.Sprintf("api/dashboards/%s", path), nil); err != nil {
		return Board{}, BoardProperties{}, err
	}
	if code != 200 {
		return Board{}, BoardProperties{}, fmt.Errorf("HTTP error %d: returns %s", code, raw)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&result); err != nil {
		return Board{}, BoardProperties{}, fmt.Errorf("unmarshal board with meta: %s\n%s", err, raw)
	}
	return result.Board, result.Meta, err
}

// GetRawDashboard loads a dashboard JSON from Grafana instance along with metadata for a dashboard.
// Contrary to GetDashboardByUID() it not unpack loaded JSON to Board structure. Instead it
// returns it as byte slice. It guarantee that data of dashboard returned untouched by conversion
// with Board so no matter how properly fields from a current version of Grafana mapped to
// our Board fields. It useful for backuping purposes when you want a dashboard exactly with
// same data as it exported by Grafana.
//
// For dashboards from a filesystem set "file/" prefix for slug. By default dashboards from
// a database assumed. Database dashboards may have "db/" prefix or may have not, it will
// be appended automatically.
func (r *Client) GetRawDashboard(slug string) ([]byte, BoardProperties, error) {
	var (
		raw    []byte
		result struct {
			Meta  BoardProperties `json:"meta"`
			Board json.RawMessage `json:"dashboard"`
		}
		code int
		err  error
	)
	slug, _ = setPrefix(slug)
	if raw, code, err = r.get(fmt.Sprintf("api/dashboards/%s", slug), nil); err != nil {
		return nil, BoardProperties{}, err
	}
	if code != 200 {
		return nil, BoardProperties{}, fmt.Errorf("HTTP error %d: returns %s", code, raw)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&result); err != nil {
		return nil, BoardProperties{}, fmt.Errorf("unmarshal board with meta: %s\n%s", err, raw)
	}
	return []byte(result.Board), result.Meta, err
}

// FoundBoard keeps result of search with metadata of a dashboard.
type FoundBoard struct {
	ID        uint     `json:"id"`
	UID       string   `json:"uid,omitempty"`
	Title     string   `json:"title"`
	URI       string   `json:"uri"`
	URL       string   `json:"url,omitempty"`
	Type      string   `json:"type"`
	Tags      []string `json:"tags"`
	IsStarred bool     `json:"isStarred"`
}

// SearchDashboards search dashboards by substring of their title. It allows restrict the result set with
// only starred dashboards and only for tags (logical OR applied to multiple tags).
func (r *Client) SearchDashboards(query string, starred bool, tags ...string) ([]FoundBoard, error) {
	var (
		raw    []byte
		boards []FoundBoard
		code   int
		err    error
	)
	u := url.URL{}
	q := u.Query()
	if query != "" {
		q.Set("query", query)
	}
	if starred {
		q.Set("starred", "true")
	}
	for _, tag := range tags {
		q.Add("tag", tag)
	}
	if raw, code, err = r.get("api/search", q); err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("HTTP error %d: returns %s", code, raw)
	}
	err = json.Unmarshal(raw, &boards)
	return boards, err
}

// SetDashboard updates existing dashboard or creates a new one.
// Set dasboard ID to nil to create a new dashboard.
// Set overwrite to true if you want to overwrite existing dashboard with
// newer version or with same dashboard title.
// Grafana only can create or update a dashboard in a database. File dashboards
// may be only loaded with HTTP API but not created or updated.
func (r *Client) SetDashboard(board Board, overwrite bool, folderId int) (StatusMessage, error) {
	return r.SetDashboard(board, overwrite, -1)
}

func (r *Client) SetDashboardWithFolderId(board Board, overwrite bool, folderId int) (StatusMessage, error) {
	var (
		isBoardFromDB bool
		newBoard      struct {
			Dashboard Board `json:"dashboard"`
			Overwrite bool  `json:"overwrite"`
			FolderId  int   `json:"folderId"`
		}
		raw  []byte
		resp StatusMessage
		code int
		err  error
	)
	if board.Slug, isBoardFromDB = cleanPrefix(board.Slug); !isBoardFromDB {
		return StatusMessage{}, errors.New("only database dashboard (with 'db/' prefix in a slug) can be set")
	}
	newBoard.Dashboard = board
	if folderId > -1 {
		newBoard.FolderId = folderId
	}
	newBoard.Overwrite = overwrite
	if !overwrite {
		newBoard.Dashboard.ID = 0
	}
	if raw, err = json.Marshal(newBoard); err != nil {
		return StatusMessage{}, err
	}
	if raw, code, err = r.post("api/dashboards/db", nil, raw); err != nil {
		return StatusMessage{}, err
	}
	if err = json.Unmarshal(raw, &resp); err != nil {
		return StatusMessage{}, err
	}
	switch code {
	case 401:
		return StatusMessage{}, fmt.Errorf("%d %s", code, *resp.Message)
	case 412:
		return StatusMessage{}, fmt.Errorf("%d %s", code, *resp.Message)
	}
	return resp, nil
}

// SetRawDashboard updates existing dashboard or creates a new one.
// Contrary to SetDashboard() it accepts raw JSON instead of Board structure.
// Grafana only can create or update a dashboard in a database. File dashboards
// may be only loaded with HTTP API but not created or updated.
func (r *Client) SetRawDashboard(raw []byte) (StatusMessage, error) {
	var (
		rawResp []byte
		resp    StatusMessage
		code    int
		err     error
		buf     bytes.Buffer
		plain   = make(map[string]interface{})
	)
	if err = json.Unmarshal(raw, &plain); err != nil {
		return StatusMessage{}, err
	}
	// TODO(axel) fragile place, refactor it
	plain["id"] = 0
	raw, _ = json.Marshal(plain)
	buf.WriteString(`{"dashboard":`)
	buf.Write(raw)
	buf.WriteString(`, "overwrite": true}`)
	if rawResp, code, err = r.post("api/dashboards/db", nil, buf.Bytes()); err != nil {
		return StatusMessage{}, err
	}
	if err = json.Unmarshal(rawResp, &resp); err != nil {
		return StatusMessage{}, err
	}
	switch code {
	case 401:
		return StatusMessage{}, fmt.Errorf("%d %s", code, *resp.Message)
	case 412:
		return StatusMessage{}, fmt.Errorf("%d %s", code, *resp.Message)
	}
	return resp, nil
}

// DeleteDashboardByUID deletes dashboard that selected by uid string.
func (r *Client) DeleteDashboardByUID(uid string) (StatusMessage, error) {
	return r.deleteDashboard("uid/" + uid)
}

// DeleteDashboardBySlug deletes dashboard that selected by slug string.
// Deprecated since Grafana v5
//
// Grafana only can delete a dashboard in a database. File dashboards
// may be only loaded with HTTP API but not deteled.
func (r *Client) DeleteDashboardBySlug(slug string) (StatusMessage, error) {
	slug, isBoardFromDB := cleanPrefix(slug)
	if !isBoardFromDB {
		return StatusMessage{}, errors.New("only database dashboards (with 'db/' prefix in a slug) can be removed")
	}
	path, _ := setPrefix(slug)
	return r.deleteDashboard(path)
}

func (r *Client) deleteDashboard(path string) (StatusMessage, error) {
	var (
		raw   []byte
		reply StatusMessage
		err   error
	)
	if raw, _, err = r.delete(fmt.Sprintf("api/dashboards/%s", path)); err != nil {
		return StatusMessage{}, err
	}
	err = json.Unmarshal(raw, &reply)
	if err != nil {
		return StatusMessage{}, err
	}
	return reply, err
}

// Permission keeps existing permissions for a dashboard.
type Permission struct {
	ID             uint      `json:"id,omitempty"`
	DashboardID    uint      `json:"dashboardId,omitempty"`
	Created        time.Time `json:"created,omitempty"`
	Updated        time.Time `json:"updated,omitempty"`
	UserID         uint      `json:"userId,omitempty"`
	UserLogin      string    `json:"userLogin,omitempty"`
	UserEmail      string    `json:"userEmail,omitempty"`
	TeamID         uint      `json:"teamId,omitempty"`
	Team           string    `json:"team,omitempty"`
	Role           string    `json:"role,omitempty"`
	Permission     uint      `json:"permission,omitempty"`
	PermissionName string    `json:"permissionName,omitempty"`
	UID            string    `json:"uid,omitempty"`
	Title          string    `json:"title,omitempty"`
	Slug           string    `json:"slug,omitempty"`
	IsFolder       bool      `json:"isFolder,omitempty"`
	URL            string    `json:"url,omitempty"`
}

// Permissions keeps list of all permissions for a dashboard.
type Permissions struct {
	Items []Permission `json:"items"`
}

// GetDashboardPermissions gets all existing permissions for the dashboard with the given id.
func (r *Client) GetDashboardPermissions(id uint) ([]Permission, error) {
	var (
		raw         []byte
		permissions []Permission
		code        int
		err         error
	)
	if raw, code, err = r.get(fmt.Sprintf("/api/dashboards/id/%d/permissions", id), nil); err != nil {
		return []Permission{}, err
	}
	if code != 200 {
		return []Permission{}, fmt.Errorf("HTTP error %d: returns %s", code, raw)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&permissions); err != nil {
		return []Permission{}, fmt.Errorf("unmarshal permissions for dashboard %d with meta: %s\n%s", id, err, raw)
	}
	return permissions, err
}

// UpdateDashboardPermissions gets all existing permissions for the dashboard with the given id.
func (r *Client) UpdateDashboardPermissions(permissions Permissions, id uint) (StatusMessage, error) {
	var (
		raw  []byte
		resp StatusMessage
		err  error
	)
	if raw, err = json.Marshal(permissions); err != nil {
		return StatusMessage{}, err
	}
	if raw, _, err = r.post(fmt.Sprintf("/api/dashboards/id/%d/permissions", id), nil, raw); err != nil {
		return StatusMessage{}, err
	}
	if err = json.Unmarshal(raw, &resp); err != nil {
		return StatusMessage{}, err
	}
	return resp, nil
}

// implicitly use dashboards from Grafana DB not from a file system
func setPrefix(slug string) (string, bool) {
	if strings.HasPrefix(slug, "db") {
		return slug, true
	}
	if strings.HasPrefix(slug, "file") {
		return slug, false
	}
	return fmt.Sprintf("db/%s", slug), true
}

// assume we use database dashboard by default
func cleanPrefix(slug string) (string, bool) {
	if strings.HasPrefix(slug, "db") {
		return slug[3:], true
	}
	if strings.HasPrefix(slug, "file") {
		return slug[3:], false
	}
	return fmt.Sprintf("%s", slug), true
}
