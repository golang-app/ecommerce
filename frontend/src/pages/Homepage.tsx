import { AppNavBar, Footer } from "../tetris";
import { Product, ProductListView } from "../productcatalog";
import { Container } from "@mui/material";

export interface HomepageProps {
  products: Product[];

  loggedIn: boolean;
}

export function Homepage(props: HomepageProps) {
  return (
    <div>
      <AppNavBar loggedIn={props.loggedIn} cartItems={[]} />
      <Container>
        <h1>Homepage</h1>
        <ProductListView products={props.products} />
      </Container>
      <Footer />
    </div>
  );
}
