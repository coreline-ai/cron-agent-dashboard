import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

type MarkdownTextProps = {
  value: string;
};

export function MarkdownText({ value }: MarkdownTextProps) {
  return (
    <div className="markdown-content">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{value}</ReactMarkdown>
    </div>
  );
}
