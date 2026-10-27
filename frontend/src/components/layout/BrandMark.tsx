import icon from "../../../../packaging/linux/cliprag.svg";
export function BrandMark({ size = 42 }: { size?: number }) {
  return (
    <img
      className="brand-mark"
      src={icon}
      width={size}
      height={size}
      alt=""
      draggable={false}
    />
  );
}
