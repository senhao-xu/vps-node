-- External chain exits: an entry node may point at a single line inside a
-- custom node source instead of a managed exit node. The two chain target
-- columns are mutually exclusive at the application layer, so no CHECK (and
-- therefore no table rebuild) is added here.
ALTER TABLE nodes ADD COLUMN chain_custom_node_id INTEGER REFERENCES custom_nodes (id);
ALTER TABLE nodes ADD COLUMN chain_custom_entry_key TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_nodes_chain_custom_node ON nodes (chain_custom_node_id);
