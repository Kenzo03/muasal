-- +goose Up
-- A node's links to the document sections it came from go with the node: the
-- sections stay, and a mistaken node from a tree draft can be deleted (§7.7).
ALTER TABLE document_section_nodes
  DROP CONSTRAINT document_section_nodes_node_id_fkey,
  ADD CONSTRAINT document_section_nodes_node_id_fkey FOREIGN KEY (node_id) REFERENCES nodes (id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE document_section_nodes
  DROP CONSTRAINT document_section_nodes_node_id_fkey,
  ADD CONSTRAINT document_section_nodes_node_id_fkey FOREIGN KEY (node_id) REFERENCES nodes (id);
