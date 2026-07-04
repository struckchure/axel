(scalar_type
  "scalar" @context
  "type" @context
  name: (type_identifier) @name) @item

(enum_type
  "enum" @context
  name: (type_identifier) @name) @item

(type_definition
  name: (type_identifier) @name) @item

(field_declaration
  name: (field_identifier) @name) @item
