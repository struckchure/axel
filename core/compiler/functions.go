package compiler

// Known function names, used to warn about a call the compiler can't recognise.
// AQL passes function calls through to Postgres verbatim — that's the escape
// hatch for the long tail of builtins and extensions — so an unrecognised name
// is only ever a warning, never an error.

// keywordFuncs are SQL keywords that are not functions. Written as a call
// (`distinct(.roles)`) they compile cleanly and then fail at the database, so
// they get a definite message rather than a hedged one.
var keywordFuncs = map[string]bool{
	"all": true, "and": true, "any": true, "as": true, "between": true,
	"case": true, "distinct": true, "else": true, "end": true, "exists": true,
	"from": true, "in": true, "is": true, "like": true, "not": true,
	"null": true, "or": true, "select": true, "some": true, "then": true,
	"when": true, "where": true,
}

// knownFuncs is the curated set of Postgres functions AQL is expected to reach
// for. It is deliberately not exhaustive — Postgres has hundreds, and misses
// here only produce a warning.
var knownFuncs = map[string]bool{
	// Aggregates (beyond aql.AggFuncs) and window functions.
	"array_agg": true, "bool_and": true, "bool_or": true, "every": true,
	"json_agg": true, "json_object_agg": true, "jsonb_agg": true,
	"jsonb_object_agg": true, "string_agg": true, "xmlagg": true,
	"dense_rank": true, "first_value": true, "lag": true, "last_value": true,
	"lead": true, "nth_value": true, "ntile": true, "percent_rank": true,
	"rank": true, "row_number": true,
	"corr": true, "covar_pop": true, "covar_samp": true, "stddev": true,
	"stddev_pop": true, "stddev_samp": true, "variance": true, "var_pop": true,
	"var_samp": true, "mode": true, "percentile_cont": true, "percentile_disc": true,

	// Conditional / comparison.
	"coalesce": true, "nullif": true, "greatest": true, "least": true, "num_nonnulls": true,
	"num_nulls": true,

	// String.
	"ascii": true, "btrim": true, "char_length": true, "character_length": true,
	"chr": true, "concat": true, "concat_ws": true, "format": true, "initcap": true,
	"left": true, "length": true, "lower": true, "lpad": true, "ltrim": true,
	"md5": true, "octet_length": true, "overlay": true, "position": true,
	"quote_ident": true, "quote_literal": true, "quote_nullable": true,
	"regexp_count": true, "regexp_instr": true, "regexp_like": true,
	"regexp_match": true, "regexp_matches": true, "regexp_replace": true,
	"regexp_split_to_array": true, "regexp_split_to_table": true, "regexp_substr": true,
	"repeat": true, "replace": true, "reverse": true, "right": true, "rpad": true,
	"rtrim": true, "split_part": true, "starts_with": true, "strpos": true,
	"substr": true, "substring": true, "to_ascii": true, "translate": true,
	"trim": true, "unistr": true, "upper": true,

	// Encoding / binary.
	"convert_from": true, "convert_to": true, "decode": true, "encode": true,
	"sha224": true, "sha256": true, "sha384": true, "sha512": true,

	// Numeric.
	"abs": true, "cbrt": true, "ceil": true, "ceiling": true, "degrees": true,
	"div": true, "exp": true, "factorial": true, "floor": true, "gcd": true,
	"lcm": true, "ln": true, "log": true, "log10": true, "min_scale": true,
	"mod": true, "pi": true, "power": true, "radians": true, "random": true,
	"round": true, "scale": true, "sign": true, "sqrt": true, "trim_scale": true,
	"trunc": true, "width_bucket": true,
	"acos": true, "asin": true, "atan": true, "atan2": true, "cos": true,
	"cot": true, "sin": true, "tan": true,

	// Date / time.
	"age": true, "clock_timestamp": true, "current_date": true, "current_time": true,
	"current_timestamp": true, "date_bin": true, "date_part": true, "date_trunc": true,
	"extract": true, "isfinite": true, "justify_days": true, "justify_hours": true,
	"justify_interval": true, "localtime": true, "localtimestamp": true,
	"make_date": true, "make_interval": true, "make_time": true, "make_timestamp": true,
	"make_timestamptz": true, "now": true, "statement_timestamp": true,
	"timeofday": true, "transaction_timestamp": true, "to_timestamp": true,

	// Formatting.
	"to_char": true, "to_date": true, "to_hex": true, "to_number": true,

	// Array.
	"array_append": true, "array_cat": true, "array_dims": true, "array_fill": true,
	"array_length": true, "array_lower": true, "array_ndims": true,
	"array_position": true, "array_positions": true, "array_prepend": true,
	"array_remove": true, "array_replace": true, "array_to_json": true,
	"array_to_string": true, "array_upper": true, "cardinality": true,
	"string_to_array": true, "trim_array": true, "unnest": true,

	// JSON / JSONB.
	"json_array_elements": true, "json_array_elements_text": true, "json_array_length": true,
	"json_build_array": true, "json_build_object": true, "json_each": true,
	"json_each_text": true, "json_extract_path": true, "json_extract_path_text": true,
	"json_object": true, "json_object_keys": true, "json_populate_record": true,
	"json_strip_nulls": true, "json_to_record": true, "json_typeof": true,
	"jsonb_array_elements": true, "jsonb_array_elements_text": true,
	"jsonb_array_length": true, "jsonb_build_array": true, "jsonb_build_object": true,
	"jsonb_each": true, "jsonb_each_text": true, "jsonb_extract_path": true,
	"jsonb_extract_path_text": true, "jsonb_insert": true, "jsonb_object": true,
	"jsonb_object_keys": true, "jsonb_path_exists": true, "jsonb_path_match": true,
	"jsonb_path_query": true, "jsonb_path_query_array": true, "jsonb_path_query_first": true,
	"jsonb_pretty": true, "jsonb_set": true, "jsonb_set_lax": true,
	"jsonb_strip_nulls": true, "jsonb_typeof": true, "row_to_json": true,
	"to_json": true, "to_jsonb": true,

	// Range.
	"isempty": true, "lower_inc": true, "lower_inf": true, "range_merge": true,
	"upper_inc": true, "upper_inf": true,

	// UUID / identity / system.
	"gen_random_bytes": true, "gen_random_uuid": true, "uuid_generate_v1": true,
	"uuid_generate_v4": true, "crypt": true, "digest": true, "hmac": true,
	"current_database": true, "current_schema": true, "current_setting": true,
	"current_user": true, "session_user": true, "set_config": true, "version": true,

	// Text search.
	"plainto_tsquery": true, "phraseto_tsquery": true, "querytree": true,
	"setweight": true, "to_tsquery": true, "to_tsvector": true, "ts_headline": true,
	"ts_rank": true, "ts_rank_cd": true, "websearch_to_tsquery": true,

	// Misc / commonly reached for.
	"generate_series": true, "nextval": true, "currval": true, "setval": true,
	"pg_typeof": true, "similarity": true, "word_similarity": true,
}
