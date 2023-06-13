import { useContext } from "react";
import { styled } from "@mui/system";
import Typography from "@mui/material/Typography";
import Button from "@mui/material/Button";
import { Price, PriceView } from "../tetris";
import { useCart } from "../cart";
import { Container, Grid } from "@mui/material";

export interface ProductViewProps {
  product: Product;
}

export interface Product {
  id: string;
  name: string;
  price: Price;
  shortDescription: string;
  thumbnailUrl?: string;
}

const Box = styled("div")({
  border: "1px solid #ccc",
  padding: "1rem",
  margin: "1rem",
});

const ReadMoreBox = styled("div")({
  paddingTop: "1rem",
});

export function ProductView(props: ProductViewProps) {
  const cart = useCart();

  console.log(cart);

  return (
    <Box>
      <Typography variant="h3">{props.product.name}</Typography>
      <Typography variant="body1">{props.product.shortDescription}</Typography>
      <Typography variant="body2">
        <PriceView price={props.product.price} />
      </Typography>
      <ReadMoreBox>
        <Button variant="contained" onClick={() => {
            cart?.addToCart(props.product.id, 1);
        }}>
          Add to cart
        </Button>
      </ReadMoreBox>
    </Box>
  );
}

interface ProductListProps {
  products: Product[];
}

const ProductListBox = styled("div")({
  paddingTop: "1rem",
});

export function ProductListView(props: ProductListProps) {
  return (
    <ProductListBox>
      <Container>
        <Grid container display="flex">
          {props.products.map((product) => (
            <Grid key={product.id} xs={4} item={true}>
              <ProductView product={product} />
            </Grid>
          ))}
        </Grid>
      </Container>
    </ProductListBox>
  );
}
