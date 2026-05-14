type BrandMarkProps = {
  size?: number;
  title?: string;
};

export function BrandMark({ size = 20, title = 'Corn Agent' }: BrandMarkProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="currentColor"
      xmlns="http://www.w3.org/2000/svg"
      role="img"
      aria-label={title}
    >
      <title>{title}</title>
      <circle cx="7" cy="7" r="1.7" />
      <circle cx="17" cy="7" r="1.7" />
      <circle cx="12" cy="12" r="2.4" />
      <circle cx="7" cy="17" r="1.7" />
      <circle cx="17" cy="17" r="1.7" />
    </svg>
  );
}
