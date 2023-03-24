import { AppNavBar, Footer } from "../tetris";
import { Product, ProductListView } from "../productcatalog";
import { Container } from "@mui/material";

export interface HomepageProps {
  products: Product[];
}

export function Homepage(props: HomepageProps) {
  return (
    <div>
      <AppNavBar />
      <Container>
        <h1>Homepage</h1>
        <ProductListView products={props.products} />
      </Container>
      <Footer />
    </div>
  );
}
