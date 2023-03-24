import { styled } from "@mui/system";

export interface Price {
  amount: number;
  currency: string;
}

interface PriceProps {
  price: Price;
}

const Box = styled("span")({});
const Amount = styled("span")({
  fontSize: "1.2rem",
  marginRight: "0.1rem",
  color: "#333",
});
const Currency = styled("span")({
  fontSize: "12px",
  color: "#666",
});

export function PriceView(props: PriceProps) {
  return (
    <Box>
      <Amount>{props.price.amount}</Amount>
      <Currency>{props.price.currency}</Currency>
    </Box>
  );
}
