import {
  GitBranch,
  TerminalSquare,
  Container,
  ShipWheel,
  Database,
  Braces,
  Link2,
  Code2,
  AlignLeft,
  type LucideIcon,
} from "lucide-react";
import type { Category } from "../types";
export const categoryIcons: Record<Category, LucideIcon> = {
  Git: GitBranch,
  Linux: TerminalSquare,
  Docker: Container,
  Kubernetes: ShipWheel,
  SQL: Database,
  JSON: Braces,
  URLs: Link2,
  Code: Code2,
  Text: AlignLeft,
};
export function categoryClass(name: string) {
  return `tone-${name.toLowerCase().replaceAll(" ", "-")}`;
}
