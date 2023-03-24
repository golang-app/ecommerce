import AppBar from "@mui/material/AppBar";
import Box from "@mui/material/Box";
import Toolbar from "@mui/material/Toolbar";
import Container from "@mui/material/Container";
import { CartIcon } from "../cart/CartIcon";

export function AppNavBar() {
  return (
    <AppBar position="static">
      <Container maxWidth="xl">
        <Toolbar disableGutters>
          <Box sx={{ flexGrow: 0 }}></Box>
          <CartIcon />
        </Toolbar>
      </Container>
    </AppBar>
  );
}
