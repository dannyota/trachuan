package build

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/validity"
)

func dateString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func (l *loader) loadValidity(ctx context.Context, corrections []source.Correction, asOf string) (validityResult, error) {
	rows, err := l.tx.Query(ctx, `SELECT key,number,number_norm,doc_type,issuer,issue_date,status_code,status_name,status_class,status_basis,raw_eff_from,raw_eff_to,eff_from,eff_to,is_consolidated,fetch_sha256,extract_sha256,text_source FROM document ORDER BY key`)
	if err != nil {
		return validityResult{}, fmt.Errorf("read validity documents: %w", err)
	}
	docs := map[string]validityDocument{}
	validation := map[string]correctionDocument{}
	for rows.Next() {
		var d validityDocument
		var issue, from, to *time.Time
		var extract *string
		if err = rows.Scan(&d.key, &d.number, &d.numberNorm, &d.docType, &d.issuer, &issue, &d.statusCode, &d.statusName, &d.initialClass, &d.basis, &d.rawFrom, &d.rawTo, &from, &to, &d.consolidated, &d.fetchHash, &extract, &d.textSource); err != nil {
			rows.Close()
			return validityResult{}, fmt.Errorf("scan validity document: %w", err)
		}
		d.issue, d.from, d.to = dateString(issue), dateString(from), dateString(to)
		d.sourceClass, _, _ = Status(d.statusCode, l.codes)
		if extract != nil {
			d.extractHash = *extract
		}
		for _, gap := range l.gaps {
			if gap.ItemKey == d.key && (gap.Kind == "invalid_date" || gap.Kind == "invalid_validity_interval") {
				d.dateGap = true
				break
			}
		}
		docs[d.key] = d
		validation[d.key] = correctionDocument{number: d.number, from: from, to: to, provisions: map[string]string{}}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return validityResult{}, fmt.Errorf("read validity documents: %w", err)
	}
	if err = l.readValidityProvisions(ctx, docs, validation); err != nil {
		return validityResult{}, err
	}
	entries, err := validateCorrections(corrections, validation)
	if err != nil {
		return validityResult{}, fmt.Errorf("validate corrections: %w", err)
	}
	relations, err := l.readValidityRelations(ctx)
	if err != nil {
		return validityResult{}, err
	}
	values := make([]validityDocument, 0, len(docs))
	for _, d := range docs {
		values = append(values, d)
	}
	result, err := runValidity(values, relations, entries, asOf)
	if err != nil {
		return validityResult{}, fmt.Errorf("derive validity: %w", err)
	}
	if err = l.writeValidity(ctx, result); err != nil {
		return validityResult{}, err
	}
	return result, nil
}

func (l *loader) readValidityProvisions(ctx context.Context, docs map[string]validityDocument, validation map[string]correctionDocument) error {
	rows, err := l.tx.Query(ctx, `SELECT d.key,p.id,p.parent_id,p.kind,p.label,p.heading,p.anchor,p.text FROM provision p JOIN document d ON d.id=p.document_id ORDER BY d.key,p.preorder`)
	if err != nil {
		return fmt.Errorf("read validity provisions: %w", err)
	}
	labels := map[int64]string{}
	for rows.Next() {
		var key, kind, label, heading, anchor, text string
		var id int64
		var parent *int64
		if err = rows.Scan(&key, &id, &parent, &kind, &label, &heading, &anchor, &text); err != nil {
			rows.Close()
			return fmt.Errorf("scan validity provision: %w", err)
		}
		name := label
		if parent != nil && labels[*parent] != "" {
			name = labels[*parent] + " " + strings.ToLower(label)
		}
		labels[id] = name
		cd := validation[key]
		cd.provisions[anchor] = text
		if name != "" {
			cd.provisions[name] = text
		}
		validation[key] = cd
		d := docs[key]
		if kind == "dieu" && validity.Select(heading) {
			if d.article != nil {
				d.article = &validity.Result{Reasons: []validity.Reason{{Kind: "multiple_effect_articles"}}}
			} else {
				r := validity.Parse(d.docType, text)
				d.article = &r
				d.anchor = anchor
				d.quote = text
			}
			docs[key] = d
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("read validity provisions: %w", err)
	}
	return nil
}
func (l *loader) readValidityRelations(ctx context.Context) ([]validityRelation, error) {
	rows, err := l.tx.Query(ctx, `SELECT source_key,target_key,kind,id FROM relation WHERE source_document_id IS NOT NULL AND target_document_id IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("read validity relations: %w", err)
	}
	var rels []validityRelation
	for rows.Next() {
		var r validityRelation
		if err = rows.Scan(&r.source, &r.target, &r.kind, &r.id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan validity relation: %w", err)
		}
		rels = append(rels, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read validity relations: %w", err)
	}
	return rels, nil
}
func (l *loader) writeValidity(ctx context.Context, result validityResult) error {
	keys := make([]string, 0, len(result.documents))
	for key := range result.documents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		d := result.documents[key]
		doc := l.docs[key]
		if doc == nil {
			return fmt.Errorf("validity document %q missing", key)
		}
		_, err := l.tx.Exec(ctx, `UPDATE document SET status_class=$1,is_current=$2,status_basis=$3,eff_from=$4::text::date,eff_to=$5::text::date,validity_conflict=$6 WHERE id=$7`, d.values.Class, d.values.Current, d.values.Basis, d.values.From, d.values.To, d.values.Conflict, doc.id)
		if err != nil {
			return fmt.Errorf("update validity %s: %w", key, err)
		}
		for index, decision := range d.history {
			before, e := json.Marshal(decision.before)
			if e != nil {
				return fmt.Errorf("encode validity before %s: %w", key, e)
			}
			after, e := json.Marshal(decision.after)
			if e != nil {
				return fmt.Errorf("encode validity after %s: %w", key, e)
			}
			var relation any
			if decision.relationID != 0 {
				relation = decision.relationID
			}
			_, e = l.tx.Exec(ctx, `INSERT INTO validity_decision(document_id,ordinal,layer,rule_version,before,after,actor_key,effect_date,relation_id,anchor,quote,artifact_sha256) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,NULLIF($7,''),NULLIF($8,'')::date,$9,NULLIF($10,''),NULLIF($11,''),NULLIF($12,''))`, doc.id, index+1, decision.layer, decision.version, before, after, decision.actor, decision.date, relation, decision.anchor, decision.quote, decision.artifact)
			if e != nil {
				return fmt.Errorf("insert validity decision %s: %w", key, e)
			}
		}
	}
	return nil
}
