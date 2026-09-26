ALTER TABLE nodes ADD COLUMN chain_node_id INTEGER REFERENCES nodes (id);

CREATE INDEX idx_nodes_chain_node ON nodes (chain_node_id);
