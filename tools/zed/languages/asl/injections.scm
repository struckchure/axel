; RLS policy predicates (`using (…)` / `with check (…)`) and inline trigger
; bodies (`do (…)`) are AQL — re-parse them with the AQL grammar so they get
; full AQL highlighting instead of a flat token blob. The captured node includes
; the surrounding parentheses, which the AQL grammar reads as a parenthesized
; expression.
((aql_block) @injection.content
  (#set! injection.language "aql"))

; A constraint's `filter <predicate>` is an unparenthesized AQL predicate.
((filter_predicate) @injection.content
  (#set! injection.language "aql"))
