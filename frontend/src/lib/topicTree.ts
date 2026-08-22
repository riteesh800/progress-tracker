import type { TopicNode } from "../api/topics.api";

/** A node is a leaf iff it currently has zero children — never by depth. */
export function isLeafNode(node: TopicNode): boolean {
  return !node.children || node.children.length === 0;
}

function rollup(node: TopicNode): TopicNode {
  const children = (node.children ?? []).map(rollup);
  if (children.length === 0) {
    const done = Boolean(node.is_completed);
    return {
      ...node,
      children,
      is_leaf: true,
      completed_leaves: done ? 1 : 0,
      total_leaves: 1,
      percent: done ? 100 : 0,
    };
  }
  const total = children.reduce((sum, child) => sum + child.total_leaves, 0);
  const completed = children.reduce((sum, child) => sum + child.completed_leaves, 0);
  return {
    ...node,
    children,
    is_leaf: false,
    is_completed: false,
    total_leaves: total,
    completed_leaves: completed,
    percent: total === 0 ? 0 : Math.round((completed / total) * 10000) / 100,
  };
}

export function insertChildTopic(
  nodes: TopicNode[],
  parentId: string,
  child: TopicNode,
): TopicNode[] {
  const leaf: TopicNode = {
    ...child,
    children: child.children ?? [],
    is_leaf: true,
    is_completed: false,
    completed_leaves: 0,
    total_leaves: 1,
    percent: 0,
  };

  function walk(list: TopicNode[]): TopicNode[] {
    return list.map((node) => {
      if (node.id === parentId) {
        return rollup({
          ...node,
          is_completed: false,
          children: [...(node.children ?? []), leaf],
        });
      }
      if (node.children?.length) {
        const next = walk(node.children);
        if (next === node.children) return node;
        return rollup({ ...node, children: next });
      }
      return node;
    });
  }

  return walk(nodes);
}
