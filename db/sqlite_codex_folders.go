package db

import (
	"database/sql"
	"sort"
	"waystone-web/models"
)

// MaxFolderDepth is how many levels folders may nest (a top-level folder is level 1).
const MaxFolderDepth = 10

// folderDepth counts the folders from the top level down to id, itself included.
func folderDepth(byID map[string]models.Folder, id string) int {
	n := 0
	for cur := id; cur != "" && n <= len(byID); n++ {
		cur = byID[cur].ParentID
	}
	return n
}

// folderHeight is the number of levels in the subtree under id, itself included.
func folderHeight(all []models.Folder, id string) int {
	h := 0
	for _, f := range all {
		if f.ParentID == id {
			if c := folderHeight(all, f.ID); c > h {
				h = c
			}
		}
	}
	return h + 1
}

const folderColumns = `id, world_id, parent_id, name, created_by, created_at, updated_at`

func scanFolder(row rowScanner) (models.Folder, error) {
	var f models.Folder
	err := row.Scan(&f.ID, &f.WorldID, &f.ParentID, &f.Name, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

func folderByID(q querier, id string) (models.Folder, error) {
	f, err := scanFolder(q.QueryRow(`SELECT `+folderColumns+` FROM codex_folders WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		err = ErrFolderNotFound
	}
	return f, err
}

func foldersOf(q querier, worldID string) ([]models.Folder, error) {
	rows, err := q.Query(`SELECT `+folderColumns+` FROM codex_folders WHERE world_id = ?`, worldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Folder{}
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := NormName(out[i].Name), NormName(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// folderInWorld checks that a page's folder ("" = top level) belongs to the page's world.
func folderInWorld(q querier, folderID, worldID string) error {
	if folderID == "" {
		return nil
	}
	if worldID == "" {
		return ErrNoFolders
	}
	f, err := folderByID(q, folderID)
	if err != nil {
		return err
	}
	if f.WorldID != worldID {
		return ErrFolderNotFound
	}
	return nil
}

// siblingNameFree reports whether no folder under parentID in the world (other than exceptID)
// already has this name by NormName.
func siblingNameFree(q querier, worldID, parentID, name, exceptID string) (bool, error) {
	all, err := foldersOf(q, worldID)
	if err != nil {
		return false, err
	}
	for _, f := range all {
		if f.ParentID == parentID && f.ID != exceptID && NormName(f.Name) == NormName(name) {
			return false, nil
		}
	}
	return true, nil
}

// GetFolders lists a world's folders ordered by name.
func (s *SQLiteStore) GetFolders(worldID string) ([]models.Folder, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return foldersOf(s.db, worldID)
}

// GetFolder returns nil, nil when the folder does not exist.
func (s *SQLiteStore) GetFolder(id string) (*models.Folder, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	f, err := folderByID(s.db, id)
	if err == ErrFolderNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// CreateFolder adds a folder under f.ParentID ("" = the world's top level). Names are unique
// among siblings; the Unsorted folder (world "") can't have folders.
func (s *SQLiteStore) CreateFolder(f models.Folder) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if f.WorldID == "" {
		return ErrNoFolders
	}
	if err := worldExists(tx, f.WorldID); err != nil {
		return err
	}
	if f.ParentID != "" {
		parent, err := folderByID(tx, f.ParentID)
		if err == ErrFolderNotFound {
			return ErrParentNotFound
		}
		if err != nil {
			return err
		}
		if parent.WorldID != f.WorldID {
			return ErrParentNotFound
		}
		all, err := foldersOf(tx, f.WorldID)
		if err != nil {
			return err
		}
		byID := map[string]models.Folder{}
		for _, x := range all {
			byID[x.ID] = x
		}
		if folderDepth(byID, f.ParentID)+1 > MaxFolderDepth {
			return ErrFolderTooDeep
		}
	}
	if free, err := siblingNameFree(tx, f.WorldID, f.ParentID, f.Name, ""); err != nil {
		return err
	} else if !free {
		return ErrFolderNameTaken
	}
	now := nowUTC()
	if _, err := tx.Exec(`INSERT INTO codex_folders (`+folderColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.WorldID, f.ParentID, f.Name, f.CreatedBy, now, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) RenameFolder(id, name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	f, err := folderByID(tx, id)
	if err != nil {
		return err
	}
	if free, err := siblingNameFree(tx, f.WorldID, f.ParentID, name, id); err != nil {
		return err
	} else if !free {
		return ErrFolderNameTaken
	}
	if _, err := tx.Exec(`UPDATE codex_folders SET name = ?, updated_at = ? WHERE id = ?`, name, nowUTC(), id); err != nil {
		return err
	}
	return tx.Commit()
}

// MoveFolder re-parents a folder inside its world ("" = top level). It can't go inside itself
// or one of its descendants.
func (s *SQLiteStore) MoveFolder(id, newParentID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	f, err := folderByID(tx, id)
	if err != nil {
		return err
	}
	all, err := foldersOf(tx, f.WorldID)
	if err != nil {
		return err
	}
	byID := map[string]models.Folder{}
	for _, x := range all {
		byID[x.ID] = x
	}
	if newParentID != "" {
		if _, ok := byID[newParentID]; !ok {
			return ErrParentNotFound
		}
		// Walk up from the target: meeting the folder itself means it is a descendant.
		for cur, hops := newParentID, 0; cur != "" && hops <= len(all); hops++ {
			if cur == id {
				return ErrFolderCycle
			}
			cur = byID[cur].ParentID
		}
	}
	if f.ParentID == newParentID {
		return nil
	}
	if newParentID != "" && folderDepth(byID, newParentID)+folderHeight(all, id) > MaxFolderDepth {
		return ErrFolderTooDeep
	}
	if free, err := siblingNameFree(tx, f.WorldID, newParentID, f.Name, id); err != nil {
		return err
	} else if !free {
		return ErrFolderNameTaken
	}
	if _, err := tx.Exec(`UPDATE codex_folders SET parent_id = ?, updated_at = ? WHERE id = ?`, newParentID, nowUTC(), id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteFolder removes only the folder row: its sub-folders and pages move up one level, into
// its parent or the world's top level. Foreign keys are off, so this is done by hand. It is
// refused (ErrFolderNameTaken) when a sub-folder's name is already used in the parent.
func (s *SQLiteStore) DeleteFolder(id string) (int, int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	f, err := folderByID(tx, id)
	if err != nil {
		return 0, 0, err
	}
	all, err := foldersOf(tx, f.WorldID)
	if err != nil {
		return 0, 0, err
	}
	taken := map[string]bool{}
	for _, x := range all {
		if x.ParentID == f.ParentID && x.ID != id {
			taken[NormName(x.Name)] = true
		}
	}
	children := 0
	for _, x := range all {
		if x.ParentID == id {
			if taken[NormName(x.Name)] {
				return 0, 0, ErrFolderNameTaken
			}
			children++
		}
	}
	now := nowUTC()
	if _, err := tx.Exec(`UPDATE codex_folders SET parent_id = ?, updated_at = ? WHERE parent_id = ? AND world_id = ?`, f.ParentID, now, id, f.WorldID); err != nil {
		return 0, 0, err
	}
	res, err := tx.Exec(`UPDATE pages SET folder_id = ? WHERE folder_id = ?`, f.ParentID, id)
	if err != nil {
		return 0, 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	if _, err := tx.Exec(`DELETE FROM codex_folders WHERE id = ?`, id); err != nil {
		return 0, 0, err
	}
	return int(n), children, tx.Commit()
}

// SetPageFolder files a page in a folder of its own world ("" = top level). Links don't change.
func (s *SQLiteStore) SetPageFolder(pageID, folderID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	world, _, err := pageWorld(tx, pageID)
	if err != nil {
		return err
	}
	if err := folderInWorld(tx, folderID, world); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE pages SET folder_id = ? WHERE id = ?`, folderID, pageID); err != nil {
		return err
	}
	return tx.Commit()
}
