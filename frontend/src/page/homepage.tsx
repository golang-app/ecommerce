import "./styles.css";
import {
  Navigation,
  Carousel,
  CatsOfMounth,
  FeaturedProducts,
  Footer,
  Search,
} from "../component";

import "bootstrap/dist/css/bootstrap.min.css";

export function Homepage() {
  return (
    <>
      <Navigation />
      <Search />
      <Carousel />
      <CatsOfMounth />
      <FeaturedProducts />
      <Footer />
    </>
  );
}
