package build

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"
)

func loadVersions(ctx context.Context, tx pgx.Tx, d databaseInputs, ids *databaseIDs) (int, error) {
	frameworks := map[string]bool{}
	for i, v := range d.versions {
		if !frameworks[v.Code] {
			aliases := []string{}
			seen := map[string]bool{}
			for _, other := range d.versions {
				if other.Code == v.Code {
					for _, alias := range other.Aliases {
						if !seen[alias] {
							aliases = append(aliases, alias)
							seen[alias] = true
						}
					}
				}
			}
			sort.Strings(aliases)
			if _, err := tx.Exec(ctx, `INSERT INTO framework(id,code,name,publisher,aliases) VALUES($1,$2,$3,$4,$5)`, len(frameworks)+1, v.Code, v.Name, v.Publisher, aliases); err != nil {
				return 0, hide("load framework", err)
			}
			frameworks[v.Code] = true
		}
		date := "-infinity"
		if v.Released != nil {
			date = *v.Released
		}
		id := int64(i + 1)
		//nolint:misspell // Match the registry field names.
		_, err := tx.Exec(ctx, `INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,licence_url,released,ordinal) VALUES($1,$2,$3,$4,$5,$6,$7,$8::date,$9)`, id, v.Code, v.Version, v.Language, v.Access, v.LicenceKind, v.LicenceURL, date, v.Ordinal)
		if err != nil {
			return 0, hide("load framework version", err)
		}
		ids.versions[versionKey(v.Code, v.Version)] = id
	}
	return len(frameworks), nil
}
