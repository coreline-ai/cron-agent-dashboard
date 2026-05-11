type MarkdownTextProps = {
  value: string;
};

export function MarkdownText({ value }: MarkdownTextProps) {
  return <pre className="markdown-fallback">{value}</pre>;
}
